package nilcheck

import (
	"testing"

	"github.com/redis/go-redis/v9"
)

// stubClient is a value type, covering the branch with nothing that could be
// nil. Test doubles are commonly written this way.
type stubClient struct{}

func TestIsNil(t *testing.T) {
	var nilClient *redis.Client
	var nilCluster *redis.ClusterClient
	var nilRing *redis.Ring
	var nilUniversal redis.UniversalClient

	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{"untyped nil", nil, true},
		{"nil interface", nilUniversal, true},
		// The case that motivates this package: a non-nil interface holding a
		// nil pointer, which plain == nil misses and pinging panics on.
		{"typed nil *redis.Client", nilClient, true},
		{"typed nil *redis.ClusterClient", nilCluster, true},
		{"typed nil *redis.Ring", nilRing, true},
		{"nil map", map[string]string(nil), true},
		{"nil slice", []string(nil), true},
		{"live client", redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"}), false},
		{"struct value", stubClient{}, false},
		{"pointer to struct", &stubClient{}, false},
		{"string", "", false},
		{"zero int", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNil(tt.value); got != tt.want {
				t.Errorf("IsNil(%#v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

// service is how a typed nil actually reaches a constructor: a struct whose
// client field was declared and never assigned a live client.
//
// The field also keeps the comparison below an honest runtime test. Assigning
// a concrete-typed variable straight into a local interface lets the compiler
// see both sides and fold the result, which is a real bug in production code
// and is what staticcheck's SA4023 reports -- so writing the premise that way
// would be asserting something the compiler had already decided.
type service struct {
	rdb redis.UniversalClient
}

// A typed nil assigned to redis.UniversalClient is the exact shape the
// constructors receive, and it is not caught by comparing the interface to nil.
func TestIsNilCatchesWhatEqualityMisses(t *testing.T) {
	var unset *redis.Client // declared, never assigned a live client
	svc := &service{rdb: unset}

	if svc.rdb == nil {
		t.Fatal("a typed nil in an interface should not compare equal to nil; the premise of this package is wrong")
	}
	if !IsNil(svc.rdb) {
		t.Error("IsNil() = false for a redis.UniversalClient holding a nil *redis.Client, want true")
	}
}
