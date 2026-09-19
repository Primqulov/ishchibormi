package jobalert

import (
	"math"
	"testing"
)

// Masofa qidiruv (internal/elon/nearby.go) bilan bir xil bo'lishi kerak:
// foydalanuvchi signalda "3.1 km" ni ko'rib, e'lonni ochganda boshqa raqamni
// ko'rsa, ikkalasiga ham ishonmay qo'yadi.
func TestDistanceMetersMatchesKnownDistance(t *testing.T) {
	// Toshkent — Samarqand: taxminan 270 km.
	got := DistanceMeters(41.3111, 69.2797, 39.6542, 66.9597)
	if got < 260000 || got > 280000 {
		t.Fatalf("Toshkent–Samarqand = %.0f m, kutilgan ~270 km", got)
	}
	if d := DistanceMeters(41.3111, 69.2797, 41.3111, 69.2797); d != 0 {
		t.Fatalf("bir xil nuqta uchun %v, 0 kutilgan", d)
	}
}

// To'rtburchak — Mongo so'rovidagi qo'pol filtr. U radius ichidagi HECH QANDAY
// nuqtani kesib tashlamasligi kerak, aks holda signal jimgina ishlamay qolardi.
func TestBoundingDeltaNeverCutsOffPointsInsideRadius(t *testing.T) {
	for _, lat := range []float64{0, 41.3, 55, -33} {
		latDelta, lngDelta := boundingDelta(lat, MaxRadiusM)
		north := DistanceMeters(lat, 0, lat+latDelta, 0)
		east := DistanceMeters(lat, 0, lat, lngDelta)
		if north < MaxRadiusM || east < MaxRadiusM {
			t.Fatalf("lat=%v: to'rtburchak radiusdan kichik (shimol %.0f m, sharq %.0f m)", lat, north, east)
		}
	}
}

// Qutb yaqinida kosinus nolga intiladi — cheklovsiz bo'lsa cheksiz qiymat
// chiqib, Mongo so'rovi buzilardi.
func TestBoundingDeltaStaysFiniteNearPoles(t *testing.T) {
	_, lngDelta := boundingDelta(89.999, MaxRadiusM)
	if math.IsInf(lngDelta, 0) || math.IsNaN(lngDelta) || lngDelta > 180 {
		t.Fatalf("qutbda yaroqsiz delta: %v", lngDelta)
	}
}

func TestDistanceTextRoundsLikeTheSearchCards(t *testing.T) {
	for _, tc := range []struct {
		meters float64
		want   string
	}{{111.2, "111 m"}, {999, "999 m"}, {1250, "1.2 km"}, {12000, "12.0 km"}} {
		if got := DistanceText(tc.meters); got != tc.want {
			t.Fatalf("DistanceText(%v) = %q, kutilgan %q", tc.meters, got, tc.want)
		}
	}
}
