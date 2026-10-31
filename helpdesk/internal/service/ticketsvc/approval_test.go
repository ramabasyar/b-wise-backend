package ticketsvc

import "testing"

func TestFlowHasSteps(t *testing.T) {
	cases := []struct {
		name string
		flow string
		want bool
	}{
		{"objek kosong", "{}", false},
		{"string kosong", "", false},
		{"array kosong", "[]", false},
		{"bukan json", "bukan-json", false},
		{"satu langkah", "[{\"step\":1,\"rule\":\"direct_supervisor\"}]", true},
		{"dua langkah", "[{\"step\":1,\"rule\":\"person\",\"user_id\":\"8\"},{\"step\":2,\"rule\":\"direct_supervisor\"}]", true},
	}
	for _, c := range cases {
		if got := flowHasSteps(c.flow); got != c.want {
			t.Errorf("flowHasSteps(%q) = %v, ingin %v (%s)", c.flow, got, c.want, c.name)
		}
	}
}
