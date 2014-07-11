// GoEconGo project main.go
package main

import (
	"fmt"
	//"math/rand"
	"runtime"
	"time"
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
//inputs - what the actual production requires (map of pointer to commodity to how
//many are needed)
//basicNeeds - what is needed for this production to go forward - without it, fail
//productionMethod (map of pointer to commodity to how many are needed)
//outputs - what is produced by this production method (map of pointer to commodity
// to how many are produced)
//consumption - what is/has a chance of being consumed by the production (map of
//pointer to commodity to probability [0.0,1.0] of it being consumed)
type productionMethod struct {
	inputs      map[*commodity]int
	basicNeeds  map[*commodity]int
	outputs     map[*commodity]int
	consumption map[*commodity]float32
}

//A productionSet is a collection of similar productionMethods for producing a
//commodity.
//methods - all of the available productionMethods in this set (slice of pointers
//to productionMethod)
//penalty - cost of not following this production set (float32)
type productionSet struct {
	methods []*productionMethod
	penalty float32
}

//A traderAgent is an independent agent.  It has a job (productionSet), an inventory,
//a belief on all the prices of commodities, and cash on hand.
//job - a pointer to productionSet
//inventory - a map of pointer to commodities to how many the agent has on hand (int)
//priceBelief - an agent's belief of the current price range of commodities
//(map of pointer to commodity to priceRange)
//funds - the amount of cash on hand
type traderAgent struct {
	job         *productionSet
	inventory   map[*commodity]int
	priceBelief map[*commodity]priceRange
	funds       float32
}


//An ask is a request to the market to sell an item at a given price.
//item - a pointer to a commodity that is being sold
//quantity - a number of units to sell
//sellFor - a price to sell that commodity at
//accepted - a channel to feed back results to the agent
type ask struct {
	item     *commodity
	quantity int
	sellFor  float32
	accepted chan bool
}

//A bid is a request to the market to buy a commodity at a given price.
//item - a pointer to a commodity that we wish to purchase
//quantity - the number of units to attempt to buy
//buyFor - a price to buy that commodity for
//accepted - a channel to feed back results to the agent
type bid struct {
	item     *commodity
	quantity int
	buyFor   float32
	accepted chan bool
}

//func agentTrader(startCash float32, job *productionSet, agentStatusOut chan<- *agentStatus,
//	agentAsk chan<- *[]ask, agentBid chan<- *[]bid) {

//	funds := startCash
//	inventory = make(map[*commodity]int)
//	priceBelief := make(map[*commodity]priceRange)
//	asks := make([]ask)
//	bids := make([]bid)
//	//Loop forever, or until we die (AKA run out of money)
//	for {
//		//First, try and perform production
//		performProduction(job, *inventory)

//		//TODO: Think about this more - we need a clean way of sending in a whole
//		//set of bids and asks, then receiving whether or not they were successful.

//		//Then, generate offers
//		asks = generateAsks(job, *inventory)
//		bids = generateBids(job, *inventory)
//		//Send the offers in
//		agentAsks <- asks
//		agentBids <- bids
//	}
//}

func main() {
	fmt.Println("Hello World!")
	fmt.Println("Set up a ticker!")
	
	ticker := time.NewTicker(time.Millisecond * 100)
	
	go func() {
		for t := range ticker.C {
			fmt.Println("tick at", t)
		}
	}()

	//Block forever
	select{}
}

//Set up our agent system/world state in here.
func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	fmt.Printf("Number of CPUS: %d\n", runtime.NumCPU())
}
