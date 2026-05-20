// Package router is the call-routing core. Given a dialer source IP and a
// dialed number, it produces a full routing decision (which client, which
// signaling IP, which carrier, which media node, which media IP).
//
// Kamailio invokes this via http_async_client on every INVITE; the result
// is cached in htable for a few seconds to absorb bursts.
package router

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Decision struct {
	ClientID         int64  `json:"client_id"`
	ClientName       string `json:"client_name"`
	SignalingIP      string `json:"signaling_ip"`
	CarrierID        int64  `json:"carrier_id"`
	CarrierHost      string `json:"carrier_host"`
	CarrierPort      int    `json:"carrier_port"`
	CarrierTransport string `json:"carrier_transport"`
	MediaNodeID      int64  `json:"media_node_id"`
	MediaIP          string `json:"media_ip"`
	RotationStrategy string `json:"rotation_strategy"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"error"`
}

func (e *Error) Error() string { return fmt.Sprintf("%d: %s", e.Code, e.Message) }

// Resolve looks up the routing decision for one call. Steps:
//   1. dialer source IP   → client (via client_ips)
//   2. client + DNIS      → carrier (via routes, longest-prefix match, priority)
//   3. client             → signaling IP (clients.signaling_ip_id)
//   4. client + carrier   → active assignment → IP group → pick member by strategy
//
// Any failure produces an Error with a SIP-friendly code so Kamailio can
// reply with the right status (403 Forbidden, 404 Not Found, 503 Busy).
func Resolve(ctx context.Context, pg *pgxpool.Pool, srcIP, dnis string) (*Decision, error) {
	// --- step 1: client by dialer source IP ---
	var (
		clientID   int64
		clientName string
		sigID      *int64
		sigIP      *string
	)
	err := pg.QueryRow(ctx, `
		SELECT c.id, c.name, c.signaling_ip_id, host(s.ip_address)
		  FROM client_ips ci
		  JOIN clients c ON c.id = ci.client_id
		  LEFT JOIN signaling_ips s ON s.id = c.signaling_ip_id
		 WHERE ci.ip_address = $1::inet
		   AND ci.status = 'active' AND c.status = 'active'
		 LIMIT 1
	`, srcIP).Scan(&clientID, &clientName, &sigID, &sigIP)
	if err != nil {
		return nil, &Error{Code: 403, Message: "dialer ip not whitelisted"}
	}
	if sigIP == nil {
		return nil, &Error{Code: 500, Message: "client has no signaling ip assigned"}
	}

	// --- step 2: pick carrier via longest-prefix match on routes ---
	// We pull all candidate routes ordered by (prefix length desc, priority asc).
	var (
		carrierID        int64
		carrierHost      string
		carrierPort      int
		carrierTransport string
	)
	err = pg.QueryRow(ctx, `
		SELECT car.id, car.host, car.port, car.transport
		  FROM routes r
		  JOIN carriers car ON car.id = r.carrier_id
		 WHERE r.client_id = $1
		   AND r.status = 'active'
		   AND car.status = 'active'
		   AND ($2 LIKE COALESCE(r.match_prefix, '') || '%' OR r.match_prefix IS NULL OR r.match_prefix = '')
		 ORDER BY length(COALESCE(r.match_prefix, '')) DESC, r.priority ASC
		 LIMIT 1
	`, clientID, dnis).Scan(&carrierID, &carrierHost, &carrierPort, &carrierTransport)
	if err != nil {
		return nil, &Error{Code: 404, Message: "no route for this client + destination"}
	}

	// --- step 3: pick a media node + IP via the active assignment ---
	var (
		assignID     int64
		groupID      int64
		strategy     string
		cursor       int
	)
	err = pg.QueryRow(ctx, `
		SELECT id, group_id, rotation_strategy, rotation_cursor
		  FROM assignments
		 WHERE client_id = $1 AND carrier_id = $2 AND status = 'active'
		 LIMIT 1
	`, clientID, carrierID).Scan(&assignID, &groupID, &strategy, &cursor)
	if err != nil {
		return nil, &Error{Code: 503, Message: "no active assignment for this client+carrier"}
	}

	// Fetch active IPs in the group, joined with their node, filtered to
	// nodes that this carrier is allowed to route via, and excluding IPs
	// at their per-IP cap.
	rows, err := pg.Query(ctx, `
		SELECT ni.id, ni.node_id, host(ni.ip_address),
		       COALESCE(ni.current_calls, 0), COALESCE(ni.max_calls, 0)
		  FROM ip_group_members m
		  JOIN node_ips ni ON ni.id = m.ip_id
		  JOIN media_nodes mn ON mn.id = ni.node_id
		 WHERE m.group_id = $1 AND m.active = true
		   AND ni.status IN ('active','reserve')
		   AND mn.status = 'online'
		   AND mn.id IN (SELECT node_id FROM carrier_media_nodes WHERE carrier_id = $2 AND status = 'active')
		 ORDER BY ni.id
	`, groupID, carrierID)
	if err != nil {
		return nil, &Error{Code: 500, Message: "ip pool query failed"}
	}
	defer rows.Close()
	type cand struct {
		IPID    int64
		NodeID  int64
		IP      string
		Current int
		Max     int
	}
	cands := []cand{}
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.IPID, &c.NodeID, &c.IP, &c.Current, &c.Max); err != nil {
			continue
		}
		if c.Max > 0 && c.Current >= c.Max {
			continue
		}
		cands = append(cands, c)
	}
	if len(cands) == 0 {
		return nil, &Error{Code: 503, Message: "no media IP available in group"}
	}

	var chosen cand
	switch strategy {
	case "random":
		chosen = cands[rand.Intn(len(cands))]
	case "sticky":
		// sticky-per-client: stable hash → same IP for same client until
		// it goes away. Simple modulo on client id.
		chosen = cands[int(clientID)%len(cands)]
	case "least_used":
		chosen = cands[0]
		for _, c := range cands[1:] {
			if c.Current < chosen.Current {
				chosen = c
			}
		}
	case "health_weighted":
		// Placeholder: same as round_robin until health scoring lands.
		fallthrough
	default: // round_robin
		idx := cursor % len(cands)
		chosen = cands[idx]
		_, _ = pg.Exec(ctx,
			`UPDATE assignments SET rotation_cursor = ($1 + 1) WHERE id = $2`,
			cursor, assignID)
	}

	return &Decision{
		ClientID:         clientID,
		ClientName:       clientName,
		SignalingIP:      *sigIP,
		CarrierID:        carrierID,
		CarrierHost:      carrierHost,
		CarrierPort:      carrierPort,
		CarrierTransport: carrierTransport,
		MediaNodeID:      chosen.NodeID,
		MediaIP:          chosen.IP,
		RotationStrategy: strategy,
	}, nil
}
