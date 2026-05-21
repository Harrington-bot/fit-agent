package render

import (
	"testing"
)

func TestWeatherCondition(t *testing.T) {
	cases := []struct {
		rain, cloud float64
		want        string
	}{
		{0, 3, "Clear"},
		{0, 15, "Partly Cloudy"},
		{0, 60, "Cloudy"},
		{0, 92, "Overcast"},
		{0.5, 10, "Rain"},
		{5, 80, "Heavy Rain"},
		{10, 0, "Heavy Rain"},
	}
	for _, c := range cases {
		got := weatherCondition(c.rain, c.cloud)
		if got != c.want {
			t.Errorf("weatherCondition(rain=%.1f, cloud=%.1f) = %q, want %q", c.rain, c.cloud, got, c.want)
		}
	}
}

func TestWindDirection(t *testing.T) {
	cases := []struct {
		deg  int
		want string
	}{
		{0, "N"},
		{45, "NE"},
		{90, "E"},
		{135, "SE"},
		{180, "S"},
		{225, "SW"},
		{270, "W"},
		{315, "NW"},
		{360, "N"},
		{337, "NNW"},
		{23, "NNE"},
		{68, "ENE"},
	}
	for _, c := range cases {
		got := windDirection(c.deg)
		if got != c.want {
			t.Errorf("windDirection(%d) = %q, want %q", c.deg, got, c.want)
		}
	}
}
