package main

import "testing"

func TestMorePermissive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    *User
		b    *User
		want bool
	}{
		{
			name: "delete access outranks write-only access",
			a:    &User{Name: "a", Namespace: "team10", PullOnly: true, DeleteAllowed: true},
			b:    &User{Name: "b", Namespace: "team10", PullOnly: false, DeleteAllowed: false},
			want: true,
		},
		{
			name: "write access outranks read-only access when delete is equal",
			a:    &User{Name: "a", Namespace: "team10", PullOnly: false, DeleteAllowed: false},
			b:    &User{Name: "b", Namespace: "team10", PullOnly: true, DeleteAllowed: false},
			want: true,
		},
		{
			name: "full access outranks read-delete access",
			a:    &User{Name: "a", Namespace: "team10", PullOnly: false, DeleteAllowed: true},
			b:    &User{Name: "b", Namespace: "team10", PullOnly: true, DeleteAllowed: true},
			want: true,
		},
		{
			name: "less permissive user does not outrank more permissive user",
			a:    &User{Name: "a", Namespace: "team10", PullOnly: true, DeleteAllowed: false},
			b:    &User{Name: "b", Namespace: "team10", PullOnly: false, DeleteAllowed: true},
			want: false,
		},
		{
			name: "equal permissions are not more permissive",
			a:    &User{Name: "a", Namespace: "team10", PullOnly: false, DeleteAllowed: false},
			b:    &User{Name: "b", Namespace: "team10", PullOnly: false, DeleteAllowed: false},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := morePermissive(tt.a, tt.b); got != tt.want {
				t.Fatalf("morePermissive(%+v, %+v) = %t, want %t", *tt.a, *tt.b, got, tt.want)
			}
		})
	}
}
