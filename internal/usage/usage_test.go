package usage

import "testing"

func Test_getHistogramMetric(t *testing.T) {
	type args struct {
		val          int
		bounds       []int
		metricPrefix string
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"1",
			args{0, []int{0, 10, 100, 1000, 10000, 100000}, "users."},
			"users.le_0",
		},
		{
			"2",
			args{10, []int{0, 10, 100, 1000, 10000, 100000}, "users."},
			"users.le_10",
		},
		{
			"3",
			args{30000, []int{0, 10, 100, 1000, 10000, 100000}, "users."},
			"users.le_100k",
		},
		{
			"4",
			args{3000000, []int{0, 10, 100, 1000, 10000, 100000, 10000000}, "users."},
			"users.le_10m",
		},
		{
			"5",
			args{300000, []int{0, 10, 100, 1000, 10000, 100000}, "users."},
			"users.le_inf",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getHistogramMetric(tt.args.val, tt.args.bounds, tt.args.metricPrefix); got != tt.want {
				t.Errorf("getHistogramMetric() = %v, want %v", got, tt.want)
			}
		})
	}
}
