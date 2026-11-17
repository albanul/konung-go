package gamestate

// LAN version scoring (LAN_KONG.EXE, docs/konung_lan.md#score-and-rating).
// A session's points come from killing other players' heroes and delivering
// amulet parts; at the session's end every participant's rating moves by how
// far their points were from the session's average.

// Score rules (0x0043B67C).
const (
	AmuletPoints = 25         // an amulet part delivered on a scoring spot (0x00437B73)
	RatingStart  = 1000       // a new account's rating
	RatingMax    = 10_000_000 // the rating is clamped to 0 … RatingMax
)

// HeroKillPoints is the score for killing another player's hero:
// max(1, 2·victim level − killer level + 1), doubled when the victim
// carries an amulet part (a category-11 item of type ≥ 10). Kills of
// hirelings and monsters are counted but score nothing (their call sites
// pass 0 points).
func HeroKillPoints(victimLevel, killerLevel int, victimHasAmulet bool) int {
	p := max(1, 2*victimLevel-killerLevel+1)
	if victimHasAmulet {
		p *= 2
	}
	return p
}

// SessionRatings applies the session's end (0x00444150) to the
// participants' ratings and points (same order): with R̄ the mean rating, P
// the sum of positive points and p̄ = P / n, each rating gains
// (max(points, 0) − p̄) · R̄ / P, clamped to 0 … RatingMax. Nothing changes
// without participants or points. The original's float rounding is not
// traced; the result is truncated toward zero.
func SessionRatings(ratings, points []int) []int {
	out := append([]int(nil), ratings...)
	n := len(ratings)
	if n == 0 || len(points) != n {
		return out
	}
	var sumR, sumP float64
	for i := range n {
		sumR += float64(ratings[i])
		sumP += float64(max(points[i], 0))
	}
	if sumP == 0 {
		return out
	}
	meanR, meanP := sumR/float64(n), sumP/float64(n)
	for i := range n {
		r := float64(ratings[i]) + (float64(max(points[i], 0))-meanP)*meanR/sumP
		out[i] = int(min(max(r, 0), RatingMax))
	}
	return out
}
