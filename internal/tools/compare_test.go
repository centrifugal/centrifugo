package tools

import "testing"

func TestSecureCompare(t *testing.T) {
	type args struct {
		given  []byte
		actual []byte
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "eq",
			args: args{
				given:  []byte("test"),
				actual: []byte("test"),
			},
			want: true,
		},
		{
			name: "ne",
			args: args{
				given:  []byte("boom"),
				actual: []byte("boot"),
			},
			want: false,
		},
		{
			name: "ne_len",
			args: args{
				given:  []byte("booms"),
				actual: []byte("boom"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SecureCompare(tt.args.given, tt.args.actual); got != tt.want {
				t.Errorf("SecureCompare() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecureCompareString(t *testing.T) {
	type args struct {
		given  string
		actual string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "eq",
			args: args{
				given:  "test",
				actual: "test",
			},
			want: true,
		},
		{
			name: "ne",
			args: args{
				given:  "boom",
				actual: "boot",
			},
			want: false,
		},
		{
			name: "ne_len",
			args: args{
				given:  "booms",
				actual: "boom",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SecureCompareString(tt.args.given, tt.args.actual); got != tt.want {
				t.Errorf("SecureCompareString() = %v, want %v", got, tt.want)
			}
		})
	}
}
