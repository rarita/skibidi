package lolmonitor

import (
	"fmt"
	"github.com/gocolly/colly"
	"log"
)

const leagueGraphsRootUrlTemplate = "https://www.leagueofgraphs.com/summoner/%s/%s/"
const leagueGraphsMatchUrlTemplate = "https://www.leagueofgraphs.com/match/euw/%d"

type LeagueMonitor struct {
	summonerName       string
	summonerRegion     string
	lastMatchId        int64
	refreshRateMinutes int

	messageTemplates []string

	collector *colly.Collector
}

// parseLatestGames Parses the latest games table for summoner
// and returns an array of their latest 10(?) games.
func (mon *LeagueMonitor) parseLatestGames(elem *colly.HTMLElement) {

	log.Printf(
		"Started parsing latest games for summoner %s on region %s...\n",
		mon.summonerName,
		mon.summonerRegion,
	)

	link, present := elem.DOM.Find("a[href]").Attr("href")
	if present {
		err := elem.Request.Visit(link)
		if err != nil {
			log.Printf("Tried to check match by link %s but got error %s", link, err)
		}
	}

}

func (mon *LeagueMonitor) parseMatch(elem *colly.HTMLElement) {
	log.Printf("Starting to parse match %s...\n", elem.Request.URL.Path)
}

// initCollector Initialize GoColly collector with all the
// required callbacks set up.
func (mon *LeagueMonitor) initCollector() (c *colly.Collector) {
	c = colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/84.0.4144.2 Safari/537.37"),
	)
	c.OnRequest(func(r *colly.Request) {
		log.Printf("Visiting %s...\n", r.URL)
	})
	c.OnHTML(".championCellLight", mon.parseLatestGames) // href~='match'
	c.OnHTML(".matchTable", mon.parseMatch)
	return
}

// update Fetch new data from League of Graphs for the specified summoner.
func (mon *LeagueMonitor) update() error {

	if mon.collector == nil {
		mon.collector = mon.initCollector()
	}

	err := mon.collector.Visit(fmt.Sprintf(leagueGraphsRootUrlTemplate, mon.summonerRegion, mon.summonerName))

	if err != nil {
		return err
	}
	return nil

}
