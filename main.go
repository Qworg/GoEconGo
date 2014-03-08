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
//methods - all of the available productionMethods in this set (slice of
//productionMethod)
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
//riskAversion - the level of look ahead in value during bidding in case of failed
//bids.  Lower is more risky (since you could blow a bid)
type traderAgent struct {
	job          *productionSet
	inventory    map[*commodity]int
	priceBelief  map[*commodity]priceRange
	funds        float32
	riskAversion int
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
	offeredAsk     ask
	numberOffered  int
	numberAccepted int
}

type bids struct {
	offeredBid     bid
	numberOffered  int
	numberAccepted int
}

//Borrowed from Andy Balholm
type sortedProductionValueMap struct {
	m  map[*productionMethod]float32
	pv []*productionMethod
}

func (sm *sortedProductionValueMap) Len() int {
	return len(sm.m)
}

func (sm *sortedProductionValueMap) Less(i, j int) bool {
	return sm.m[sm.pv[i]] > sm.m[sm.pv[j]]
}

func (sm *sortedProductionValueMap) Swap(i, j int) {
	sm.pv[i], sm.pv[j] = sm.pv[j], sm.pv[i]
}

func sortedPVKeys(m map[*productionMethod]float32) []*productionMethod {
	sm := new(sortedProductionValueMap)
	sm.m = m
	sm.pv = make([]*productionMethod, len(m))
	i := 0
	for key, _ := range m {
		sm.pv[i] = key
		i++
	}
	sort.Sort(sm)
	return sm.pv
}

//commodityQuantity map concat
func cQMapConcat(mA map[*commodity]int, mB map[*commodity]int) map[*commodity]int {
	//This performs a deep concat of two *commodity -> int maps, adding the ints
	//together if they exist, while adding the keys that don't.
	mOut := mA

	for k, v := range mB {
		mOut[k] = mOut[k] + v
	}

	return mOut

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

//This is the definition of the sort for market value sorting.
type ByMarketValue []*productionMethod

func (a ByMarketValue) Len() int           { return len(a) }
func (a ByMarketValue) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByMarketValue) Less(i, j int) bool { return getMarketValue(a[i]) < getMarketValue(a[j]) }

//This is a market value calculator for a particular production method.  It calculates
//it purely from public information.
func getMarketValue(method *productionMethod) float32 {
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
		expectedValue = expectedValue - float32(catalysts.quantity)*method.consumption[index]*
			catalysts.item.averagePrice
	}

	return expectedValue
}

//This generates the average process value for a particular productionNumber of
//agent productions.  This is calculated by averaging the agent's high and low price
//values.
func getAverageProductionValue(agent *traderAgent, productionNumber int) float32 {
	var productionValue float32 = 0
	if productionNumber >= len(agent.job.methods) {
		//ERROR!  Production number is out of bounds.
		return -1
	}
	method := agent.job.methods[productionNumber]
	//Get the upside
	for _, outputs := range method.outputs {
		productionValue = productionValue + float32(outputs.quantity)*
			((agent.priceBelief[outputs.item].high+agent.priceBelief[outputs.item].low)/2)
	}
	//Calculate the cost of inputs and subtract
	for _, inputs := range method.inputs {
		productionValue = productionValue - float32(inputs.quantity)*
			((agent.priceBelief[inputs.item].high+agent.priceBelief[inputs.item].low)/2)
	}
	//Calculate the catalyst costs and subtract
	for index, catalysts := range method.catalysts {
		productionValue = productionValue - float32(catalysts.quantity)*method.consumption[index]*
			((agent.priceBelief[catalysts.item].high+agent.priceBelief[catalysts.item].low)/2)
	}

	return productionValue
}

func getAllAverageProductionValues(agent *traderAgent) map[*productionMethod]float32 {
	pvm := make(map[*productionMethod]float32)

	for index, method := range agent.job.methods {
		pvm[method] = getAverageProductionValue(agent, index)
	}

	return pvm

}

//performProduction handles the production of the agent
//Given a production set, which contains a set of production methods, the agent
//solves for the most expected value, given their internal belief of the commodity
//price.  If they cannot execute the activity with the most expected value, they
//execute the next highest value activity.  Idle agents are fined the idle penalty
//of their productionSet.
func performProduction(agent *traderAgent) {
	//This is a sorting of methods by market value.
	//BUG: This is incorrect.  However, I will test with an incorrect assumption
	//and fix it going forward.
	sort.Sort(ByMarketValue(agent.job.methods))
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

//gatherAllRequirements takes an agent's job list and returns a set of requirements
//from all of them.
//These requirements are the minimum necessary to do all the agent's jobs.
//agent - a pointer to a traderAgent dataset
//commodityNeeds - a map of commodity pointers to quantity in int
func gatherAllRequirements(agent *traderAgent) map[*commodity]int {
	commodityNeeds := make(map[*commodity]int)

	for _, method := range agent.job.methods {
		for _, inputs := range method.inputs {
			commodityNeeds[inputs.item] = commodityNeeds[inputs.item] + inputs.quantity
		}
		for _, catalysts := range method.catalysts {
			commodityNeeds[catalysts.item] = commodityNeeds[catalysts.item] + catalysts.quantity
		}
	}

	return commodityNeeds
}

//gatherRequirements takes a particular job and returns a set of requirements to
//complete that job.
func gatherRequirements(pm *productionMethod) map[*commodity]int {
	pmn := make(map[*commodity]int)

	for _, inputs := range pm.inputs {
		pmn[inputs.item] = pmn[inputs.item] + inputs.quantity
	}
	for _, catalysts := range pm.catalysts {
		pmn[catalysts.item] = pmn[catalysts.item] + catalysts.quantity
	}

	return pmn
}

//generateAsks creates asks for the agent to place in the marketplace and sell its
//goods.  These asks are based on the agent's current belief of the price modulated
//by the current price average.
//agent - a pointer to a traderAgent dataset
//askSlice - a return slice of asks.  This contains all of the asks the trader will
//make in this round of trading.
func generateAsks(agent *traderAgent) []asks {
	var askSlice []asks
	//gather any possible requirements for production
	cnm := gatherAllRequirements(agent)

	//sell everything else in inventory
	for com, num := range agent.inventory {
		_, ok := cnm[com]
		//ok is false if this inventory item is not in required items.
		//That means we should try and sell it.
		if !ok {
			var askBuild asks
			askBuild.numberAccepted = 0
			askBuild.numberOffered = num
			askBuild.offeredAsk.item = com
			//So, given the average price on the exchange, what should we sell for?
			//This instantiation sells for the average of my price belief and the
			//exchange average.
			askBuild.offeredAsk.sellFor = (agent.priceBelief[com].high + agent.priceBelief[com].low +
				com.averagePrice) / 3
			askSlice = append(askSlice, askBuild)
		}
	}

	return askSlice
}

//generateBids creates bids for the agent to place in the marketplace and buy more
//goods.  These bids are based on the agent's current belief of the price modulated
//by the current price average.
//agent - a pointer to a traderAgent dataset
//bidSlice - a return slice of asks.  This contains all of the bids the trader will
//make in this round of trading.
func generateBids(agent *traderAgent) []bids {
	var bidSlice []bids

	//Trader asks themselves what will make them the most money.
	pvm := getAllAverageProductionValues(agent)
	spv := sortedPVKeys(pvm)

	//Take the top "riskAversion" number of possible production methods and make
	//sure we can cover at least two cycles with them.
	cyclesToCover := 2
	invReqs := make(map[*commodity]int)
	for i := 0; i < agent.riskAversion; i++ {
		for j := 0; j < cyclesToCover; j++ {
			invReqs = cQMapConcat(gatherRequirements(spv[i]), invReqs)
		}
	}

	//Now that we know what we need, let's see remove what we've already got.
	for com, num := range agent.inventory {
		_, ok := invReqs[com]
		if ok {
			invReqs[com] = invReqs[com] - num
		}
	}

	//Now trimmed, let's bid for all the stuff in invReqs
	for com, num := range invReqs {
		var bidBuild bids
		bidBuild.numberOffered = num
		bidBuild.offeredBid.item = com
		//So, given the average price on the exchange, what should we buy at?
		//This instantiation buys at the average of my price belief and the
		//exchange average.
		bidBuild.offeredBid.buyFor = (agent.priceBelief[com].high + agent.priceBelief[com].low +
			com.averagePrice) / 3
		bidSlice = append(bidSlice, bidBuild)
	}

	return bidSlice
}

//TODO: OK, this is the beginning of the next part to work on.  I need to finish
//agentUpdate, as well as create the auction house. I then need to make them all
//play nice with each other over channels (as shown)

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
