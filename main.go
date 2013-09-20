// GoEconGo project main.go
package main

import (
	"fmt"
	//"math/rand"
	"runtime"
)

//A commodity is traded by traderAgents and used in production sets.
//name - name of the commodity
//averagePrice - current average price of the commodity
type commodity struct {
	name         string
	averagePrice float32
}

//A priceRange simply captures the low and high price beliefs of an agent
type priceRange struct {
	low  float32
	high float32
}

//A productionMethod defines how a commodity may be produced.
//A productionMethod is fixed at the beginning of the run.
//inputs - what the actual production requires (map of commodity to how many are needed)
//basicNeeds - what is needed for this production to go forward - without it, fail productionMethod (map of commodity to how many are needed)
//outputs - what is produced by this production method (map of commodity to how many are produced)
//consumption - what is/has a chance of being consumed by the production (map of commodity to probability [0.0,1.0] of it being consumed)
type productionMethod struct {
	inputs      map[commodity]int
	basicNeeds  map[commodity]int
	outputs     map[commodity]int
	consumption map[commodity]float32
}

//A productionSet is a collection of similar productionMethods for producing a commodity.
//methods - all of the available productionMethods in this set (slice of productionMethod)
//penalty - cost of not following this production set (float32)
type productionSet struct {
	methods []productionMethod
	penalty float32
}

//A traderAgent is an independent agent.  It has a job (productionSet), an inventory,
//a belief on all the prices of commodities, and cash on hand.
//job - a productionSet
//inventory - a map of commodities to how many the agent has on hand
//priceBelief - an agent's belief of the current price range of commodities (map of commodity to priceRange)
//funds - the amount of cash on hand
type traderAgent struct {
	job         productionSet
	inventory   map[commodity]int
	priceBelief map[commodity]priceRange
	funds       float32
}

func main() {
	fmt.Println("Hello World!")
}

//Set up our agent system/world state in here.
func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	fmt.Printf("Number of CPUS: %d\n", runtime.NumCPU())
}
