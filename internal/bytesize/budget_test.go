package bytesize

import "testing"

func TestParseBudget(t *testing.T) {
	const noDefault = 4096

	tests := []struct {
		raw     string
		want    int64
		wantErr bool
	}{
		{raw: "100mb", want: 100 << 20},
		{raw: "512", want: 512},
		{raw: "none", want: 0},
		{raw: "Unlimited", want: 0},
		{raw: " off ", want: 0},
		{raw: "0", want: 0},
		{raw: "later", wantErr: true},
		{raw: "-5", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseBudget(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseBudget(%q) error = %v, wantErr %t", tt.raw, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if limit := got.Or(noDefault); limit != tt.want {
				t.Fatalf("ParseBudget(%q).Or(%d) = %d, want %d", tt.raw, noDefault, limit, tt.want)
			}
		})
	}
}

func TestBudgetZeroValueTakesTheDefault(t *testing.T) {
	var b Budget
	if b.Set() {
		t.Fatal("the zero value reports itself as configured")
	}
	if got := b.Or(4096); got != 4096 {
		t.Fatalf("Or(4096) = %d, want the default", got)
	}
	if got := Of(0).Or(4096); got != 0 {
		t.Fatalf("Of(0).Or(4096) = %d, want no limit", got)
	}
}
