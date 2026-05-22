package api

import (
	"context"
	"log/slog"
	"time"
)

// StartCurrentCallsReconciler periodically rebuilds node_ips.current_calls
// from the authoritative source — the count of FRESH active_calls per media
// IP. This is the safety net for F2: the +1/-1 counter on node_ips can drift
// whenever a call's +1 (call-start) lands but its -1 (call-end) never fires —
// e.g. Kamailio restarts mid-call, or a BYE is lost. The drift is monotonic
// (only ever upward), so over time current_calls climbs past max_calls on
// every IP, at which point the router rejects 100% of calls with
// 503 "no media IP available in group". That exact outage was observed in
// production (one IP read 1307 with a cap of 30). This reconcile makes the
// counter self-heal every minute so a lost decrement can never accumulate
// into a routing outage.
func (s *Server) StartCurrentCallsReconciler(ctx context.Context) {
	go func() {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.reconcileCurrentCalls(ctx)
			}
		}
	}()
}

// reconcileCurrentCalls sets every node_ips.current_calls to the number of
// fresh active_calls currently relayed through that IP. Idempotent and cheap:
// the `IS DISTINCT FROM` guard means only rows that actually drifted get
// written. Bounded by its own short timeout so it can never pin a connection.
func (s *Server) reconcileCurrentCalls(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tag, err := s.deps.PG.Exec(cctx, `
		WITH fresh AS (
		    SELECT host(media_ip) AS ip, count(*) AS c
		      FROM active_calls
		     WHERE last_seen_at > now() - interval '2 minutes'
		       AND media_ip IS NOT NULL
		     GROUP BY 1
		)
		UPDATE node_ips ni
		   SET current_calls = COALESCE(f.c, 0)
		  FROM (
		    SELECT ni2.id AS id, fr.c AS c
		      FROM node_ips ni2
		      LEFT JOIN fresh fr ON fr.ip = host(ni2.ip_address)
		  ) f
		 WHERE ni.id = f.id
		   AND ni.current_calls IS DISTINCT FROM COALESCE(f.c, 0)
	`)
	if err != nil {
		slog.Error("reconcile current_calls failed", "err", err)
		return
	}
	if n := tag.RowsAffected(); n > 0 {
		slog.Info("reconciled node_ips.current_calls", "rows_corrected", n)
	}
}
