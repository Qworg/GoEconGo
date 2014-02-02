// GoEconGo project main.go
package main

import (
	"fmt"
	"math/rand"
	"runtime"
	"sort"
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

//A commoditySet simply is a number of the same commodity
type commoditySet struct {
	item     *commodity
	quantity int
}

//A productionMethod defines how a commodity may be produced.
//A productionMethod is fixed at the beginning of the run.
//inputs - what the actual production requires (a slice of commoditySets).  This is
//automatically consumed.  Without it, fail.
//catalysts - a prerequisite of an advanced production - without it, fail.  This is
//not automatically consumed. (a slice of commoditySets)
//outputs - what is produced by this production method (a slice of commoditySets)
//consumption - the chance of a catalyst being consumed by the production (an slice
//of probability [0.0,1.0] of it being consumed, aligned with the catalysts slice)
type productionMethod struct {
	inputs      []commoditySet
	catalysts   []commoditySet
	outputs     []commoditySet
	consumption []float32
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

type ask struct {
	item    *commodity
	sellFor float32
}

type bid struct {
	item   *commodity
	buyFor float32
}

type asks struct {
	offeredAsk     *ask
	numberOffered  int
	numberAccepted int
}

type bids struct {
	offeredBid     *bid
	numberOffered  int
	numberAccepted int
}

//agentRun is the execution part of the traderAgent struct.
//It performs production, sets up bids and asks, receives data back, updates
//inventories and cash on hand and updates beliefs.
//agent - a traderAgent struct
//agentAsks - a channel for asks
//agentBids - a channel for bids
//agentAlive - a channel for saying whether or not we're alive
func agentRun(agent traderAgent, agentAsks chan *[]asks, agentBids chan *[]bids, agentAlive chan bool) {
	var askSlice []asks
	var bidSlice []bids
	alive := true
	//Loop forever, until we quit or die (AKA run out of money)
	for alive {
		//First, try and perform production
		performProduction(&agent)
		//Then, generate offers
		askSlice = generateAsks(&agent)
		bidSlice = generateBids(&agent)
		//Send the offers in
		agentAsks <- &askSlice
		agentBids <- &bidSlice
		//Receive responses
		askSlice = *<-agentAsks
		bidSlice = *<-agentBids
		//Update cash on hand, inventory, and belief
		agentUpdate(&agent, &askSlice, &bidSlice)
		//If cash is gone, break the loop
		if agent.funds <= 0 {
			alive = false
		}
	}
	//Inform the world that we are dead (out of money) and return
	agentAlive <- alive
}

//This is the definition of the sort for expected value sorting.
type ByExpectedValue []*productionMethod

func (a ByExpectedValue) Len() int           { return len(a) }
func (a ByExpectedValue) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByExpectedValue) Less(i, j int) bool { return getExpectedValue(a[i]) < getExpectedValue(a[j]) }

//This is the heavy lifter of the sorting algo.  I expect this probably will need
//to be rewritten. NOTE: THIS USES AVERAGE MARKET PRICE.  THIS IS PROBABLY NOT
//CORRECT.  IT SHOULD REFLECT BELIEF OF AGENT.
func getExpectedValue(method *productionMethod) float32 {
	var expectedValue float32 = 0
	//Get the upside
	for _, outputs := range method.outputs {
		expectedValue = expectedValue + float32(outputs.quantity)*outputs.item.averagePrice
	}
	//Calculate the cost of inputs and subtract
	for _, inputs := range method.inputs {
		expectedValue = expectedValue - float32(inputs.quantity)*inputs.item.averagePrice
	}
	//Calculate the catalyst costs and subtract
	for index, catalysts := range method.catalysts {
		expectedValue = expectedValue - float32(catalysts.quantity)*method.consumption[index]*catalysts.item.averagePrice
	}

	return expectedValue
}

//performProduction handles the production of the agent
//Given a production set, which contains a set of production methods, the agent
//solves for the most expected value, given their internal belief of the commodity
//price.  If they cannot execute the activity with the most expected value, they
//execute the next highest value activity.  Idle agents are fined the idle penalty
//of their productionSet.
func performProduction(agent *traderAgent) {
	//This is a sorting of methods by expected value.
	sort.Sort(ByExpectedValue(agent.job.methods))
	//Attempt to execute methods in order of expected value.  If failing to execute,
	//apply penalty.
	accepted := false
	executedIndex := -1
	for methodIndex, method := range agent.job.methods {
		accepted = true
		for _, input := range method.inputs {
			//Make sure we have all the inputs in quantity necessary.
			//NOTE: The following construct says "accepted is equal to the current
			//truth of accepted ANDed with the truth of whether required input
			//quantity is less than or equal to what the agent has in inventory"
			accepted = accepted && input.quantity <= agent.inventory[input.item]
		}
		for _, catalyst := range method.catalysts {
			//Make sure we have all the catalysts in quantity necessary.
			accepted = accepted && catalyst.quantity <= agent.inventory[catalyst.item]
		}
		if accepted {
			executedIndex = methodIndex
			break
		}
	}
	if executedIndex == -1 {
		//Penalty!
		agent.funds = agent.funds - agent.job.penalty
	} else {
		//SUCCESS!  Work it!
		//Remove inputs!
		for _, input := range agent.job.methods[executedIndex].inputs {
			//Remove these automatically!
			agent.inventory[input.item] = agent.inventory[input.item] - input.quantity
		}
		//Try and remove catalysts!
		for catalystIndex, catalyst := range agent.job.methods[executedIndex].catalysts {
			//Test seperately for each catalyst
			for i := 0; i < catalyst.quantity; i++ {
				//Remove these on probablility given in consumption
				if agent.job.methods[executedIndex].consumption[catalystIndex] > rand.Float32() {
					//OK, you were unlucky!
					agent.inventory[catalyst.item] = agent.inventory[catalyst.item] - 1
				}
			}
		}
		//Provide output!
		for _, output := range agent.job.methods[executedIndex].outputs {
			agent.inventory[output.item] = agent.inventory[output.item] + output.quantity
		}
	}
}

//generateAsks creates asks for the agent to place in the marketplace and sell its
//goods.  These asks are based on the agent's current belief of the price modulated
//by the current price average.
//agent - a pointer to a traderAgent dataset
//askSlice - a return slice of asks.  This contains all of the asks the trader will
//make in this round of trading.
func generateAsks(agent *traderAgent) []asks {
	var askSlice []asks

	return askSlice
}

func generateBids(agent *traderAgent) []bids {
	var bidSlice []bids

	return bidSlice
}

func agentUpdate(agent *traderAgent, askSlice *[]asks, bidSlice *[]bids) {

}

func main() {
	fmt.Println("Hello World!")
}

//Set up our agent system/world state in here.
func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	fmt.Printf("Number of CPUS: %d\n", runtime.NumCPU())
	rand.Seed(time.Now().UTC().UnixNano())
}
