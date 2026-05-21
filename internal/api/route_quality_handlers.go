package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// CarrierQuality is the per-carrier scorecard over a recent window. All
// metrics are CDR-derived (no new collection); the page that surfaces this
// is "Privacy Monitor → Route quality".
//
// The composite Grade is a coarse A–F label that bakes in ASR, ACD, and
// the cause-code mix. Heuristics, not proof — passive observation can't
// definitively distinguish Tier 1 from a grey route; only active probing
// can. See Privacy page footnotes.
type CarrierQuality struct {
	CarrierID    int64   `json:"carrier_id"`
	CarrierName  string  `json:"carrier_name"`
	Total        int64   `json:"total"`
	Answered     int64   `json:"answered"`
	ASRPct       float64 `json:"asr_pct"`
	ACDSeconds   float64 `json:"acd_seconds"`
	CauseMix     map[string]int64 `json:"cause_mix"` // "200", "486", "487", "480", "503", "other"
	Grade        string  `json:"grade"`         // A / B / C / D / F
	GradeReasons []string `json:"grade_reasons"`
}

// GET /api/v1/route-quality?window=24h
func (s *Server) routeQuality(c *gin.Context) {
	win := c.DefaultQuery("window", "24h")
	dur, err := time.ParseDuration(win)
	if err != nil || dur <= 0 || dur > 30*24*time.Hour {
		c.JSON(http.StatusBadRequest, gin.H{"error": "window must be a duration like 1h, 24h, 7d (max 30d)"})
		return
	}

	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT
		    car.id, car.name,
		    count(cr.*) AS total,
		    count(*) FILTER (WHERE cr.disposition = 'answered') AS answered,
		    COALESCE(avg(cr.duration_sec) FILTER (WHERE cr.disposition = 'answered'), 0) AS acd,
		    count(*) FILTER (WHERE cr.sip_code = 200) AS c200,
		    count(*) FILTER (WHERE cr.sip_code = 486) AS c486,
		    count(*) FILTER (WHERE cr.sip_code = 487) AS c487,
		    count(*) FILTER (WHERE cr.sip_code = 480) AS c480,
		    count(*) FILTER (WHERE cr.sip_code = 503) AS c503,
		    count(*) FILTER (WHERE cr.sip_code IS NOT NULL
		                     AND cr.sip_code NOT IN (200,486,487,480,503)) AS cother
		  FROM carriers car
		  LEFT JOIN call_records cr
		    ON cr.carrier_id = car.id
		   AND cr.started_at >= now() - $1::interval
		 WHERE car.status != 'deleted'
		 GROUP BY car.id, car.name
		 ORDER BY total DESC NULLS LAST, car.name ASC
	`, dur.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	out := []CarrierQuality{}
	for rows.Next() {
		var q CarrierQuality
		var c200, c486, c487, c480, c503, cother int64
		var acd float64
		if err := rows.Scan(&q.CarrierID, &q.CarrierName, &q.Total, &q.Answered, &acd,
			&c200, &c486, &c487, &c480, &c503, &cother); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		q.ACDSeconds = acd
		if q.Total > 0 {
			q.ASRPct = float64(q.Answered) / float64(q.Total) * 100
		}
		q.CauseMix = map[string]int64{
			"200": c200, "486": c486, "487": c487, "480": c480, "503": c503, "other": cother,
		}
		q.Grade, q.GradeReasons = gradeCarrier(q)
		out = append(out, q)
	}
	c.JSON(http.StatusOK, out)
}

// gradeCarrier turns the raw metrics into a coarse A–F grade plus the
// reasons that pulled it down. We intentionally avoid hard-coding magic
// constants in the UI so operators can tune this in one place.
//
// Scoring is additive: start at 100, subtract penalties, then map to grade.
// Carriers with too little data are graded "—" (insufficient).
func gradeCarrier(q CarrierQuality) (string, []string) {
	if q.Total < 20 {
		return "—", []string{"insufficient sample (need 20+ calls)"}
	}
	score := 100
	var reasons []string

	// ASR signals. Healthy retail wholesale lands roughly 25–60%.
	switch {
	case q.ASRPct < 5:
		score -= 35
		reasons = append(reasons, "ASR very low (<5%)")
	case q.ASRPct < 15:
		score -= 20
		reasons = append(reasons, "ASR low (<15%)")
	case q.ASRPct > 90:
		// Suspiciously perfect — often a SIM box answering everything
		// to fake completion (it's a fraud indicator, not a quality win).
		score -= 25
		reasons = append(reasons, "ASR suspiciously high (>90%) — possible false-answer")
	}

	// ACD signals.
	switch {
	case q.ACDSeconds < 10 && q.Answered > 5:
		score -= 25
		reasons = append(reasons, "ACD very short (<10s) — premature drops or SIM-box detection")
	case q.ACDSeconds < 20 && q.Answered > 5:
		score -= 10
		reasons = append(reasons, "ACD short (<20s)")
	}

	// Cause-code patterns.
	if q.Total > 0 {
		p480 := float64(q.CauseMix["480"]) / float64(q.Total) * 100
		p503 := float64(q.CauseMix["503"]) / float64(q.Total) * 100
		pother := float64(q.CauseMix["other"]) / float64(q.Total) * 100
		if p480 > 30 {
			score -= 20
			reasons = append(reasons, "high 480 Temporarily Unavailable (>30%)")
		}
		if p503 > 15 {
			score -= 15
			reasons = append(reasons, "high 503 Service Unavailable (>15%) — carrier capacity issue")
		}
		if pother > 20 {
			score -= 10
			reasons = append(reasons, "high rate of non-standard cause codes (>20%)")
		}
	}

	if score < 0 {
		score = 0
	}
	var grade string
	switch {
	case score >= 90:
		grade = "A"
	case score >= 75:
		grade = "B"
	case score >= 60:
		grade = "C"
	case score >= 40:
		grade = "D"
	default:
		grade = "F"
	}
	if len(reasons) == 0 {
		reasons = []string{"clean profile"}
	}
	return grade, reasons
}
