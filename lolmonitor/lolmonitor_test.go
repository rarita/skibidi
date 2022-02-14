package lolmonitor

import (
	"testing"
)

func TestName(t *testing.T) {
	mon := LeagueMonitor{summonerName: "Killisha", summonerRegion: "euw", refreshRateMinutes: 30}
	err := mon.update()
	if err != nil {
		t.Errorf("Error during update: %s", err)
	}
}
