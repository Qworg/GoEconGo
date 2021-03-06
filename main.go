// GoEconGo project main.go
package main

import (
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sort"
	"time"
)

//Flags!
var grantGoods bool

//A commodity is traded by traderAgents and used in production sets.
//name - name of the commodity
//averagePrice - current average price of the commodity
type commodity struct {
	name         string
	averagePrice float64
}

//A priceRange simply captures the low and high price beliefs of an agent
type priceRange struct {
	low  float64
	high float64
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
	consumption []float64
}

//A productionSet is a collection of similar productionMethods for producing a
//commodity.
//methods - all of the available productionMethods in this set (slice of
//productionMethod)
//penalty - cost of not following this production set (float64)
type productionSet struct {
	methods []*productionMethod
	penalty float64
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
	role         string
	id           uint32
	job          *productionSet
	inventory    map[*commodity]int
	priceBelief  map[*commodity]priceRange
	funds        float64
	riskAversion int
}

//An ask is a request to the market to sell an item at a given price.
//item - a pointer to a commodity that is being sold
//quantity - a number of units to sell in this ask
//sellFor - a price to sell that commodity at
//accepted - whether or not this ask was successful //a channel to feed back results to the agent
type ask struct {
	id       uint64
	item     *commodity
	quantity int
	sellFor  float64
}

//A bid is a request to the market to buy a commodity at a given price.
//item - a pointer to a commodity that we wish to purchase
//quantity - the number of units to attempt to buy in this bid
//buyFor - a price to buy that commodity for
//accepted - whether or not this bid was successful //a channel to feed back results to the agent
type bid struct {
	id       uint64
	item     *commodity
	quantity int
	buyFor   float64
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
	m  map[*productionMethod]float64
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

func sortedPVKeys(m map[*productionMethod]float64) []*productionMethod {
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
		_, ok := mOut[k]
		if ok {
			mOut[k] = mOut[k] + v
		} else {
			mOut[k] = v
		}
	}
	return mOut
}

//agentRun is the execution part of the traderAgent struct.
//It performs production, sets up bids and asks, receives data back, updates
//inventories and cash on hand and updates beliefs.
//agent - a traderAgent struct
//agentAsks - a channel for asks
//agentBids - a channel for bids
//deadAgent - a channel for returning a dead traderAgent for examination and ressurection
func agentRun(agent traderAgent) (chan []asks, chan []bids, chan traderAgent) {
	var askSlice []asks
	var bidSlice []bids
	agentAsks := make(chan []asks)
	agentBids := make(chan []bids)
	deadAgent := make(chan traderAgent)
	alive := true
	go func() {
		//Loop forever, until we quit or die (AKA run out of money)
		for alive {
			//First, try and perform production
			performProduction(&agent)
			//Then, generate offers
			askSlice = nil
			bidSlice = nil
			askSlice = generateAsks(&agent)
			bidSlice = generateBids(&agent)
			//fmt.Println(askSlice)
			//Send the offers in
			agentAsks <- askSlice
			agentBids <- bidSlice
			//Receive responses
			askSlice = <-agentAsks
			//for len(askSlice) == 0 {
			//	askSlice = <-agentAsks //get the last one?
			//}
			bidSlice = <-agentBids
			//fmt.Println("Got my responses!")
			//Update cash on hand, inventory, and belief
			agentUpdate(&agent, &askSlice, &bidSlice)
			//If cash is gone, break the loop
			if agent.funds <= 0 {
				alive = false
			}
		}
		//Inform the world that we are dead (out of money) and return
		deadAgent <- agent
	}()
	return agentAsks, agentBids, deadAgent
}

//This is the definition of the sort for market value sorting.
type ByMarketValue []*productionMethod

func (a ByMarketValue) Len() int           { return len(a) }
func (a ByMarketValue) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByMarketValue) Less(i, j int) bool { return getMarketValue(a[i]) < getMarketValue(a[j]) }

//This is a market value calculator for a particular production method.  It calculates
//it purely from public information.
func getMarketValue(method *productionMethod) float64 {
	var expectedValue float64 = 0
	//Get the upside
	for _, outputs := range method.outputs {
		expectedValue = expectedValue + float64(outputs.quantity)*outputs.item.averagePrice
	}
	//Calculate the cost of inputs and subtract
	for _, inputs := range method.inputs {
		expectedValue = expectedValue - float64(inputs.quantity)*inputs.item.averagePrice
	}
	//Calculate the catalyst costs and subtract
	for index, catalysts := range method.catalysts {
		expectedValue = expectedValue - float64(catalysts.quantity)*method.consumption[index]*
			catalysts.item.averagePrice
	}

	return expectedValue
}

//This generates the average process value for a particular productionNumber of
//agent productions.  This is calculated by averaging the agent's high and low price
//values.
func getAverageProductionValue(agent *traderAgent, productionNumber int) float64 {
	var productionValue float64 = 0
	if productionNumber >= len(agent.job.methods) {
		//ERROR!  Production number is out of bounds.
		return -1
	}
	method := agent.job.methods[productionNumber]
	//Get the upside
	for _, outputs := range method.outputs {
		productionValue = productionValue + float64(outputs.quantity)*
			((agent.priceBelief[outputs.item].high+agent.priceBelief[outputs.item].low)/2)
	}
	//Calculate the cost of inputs and subtract
	for _, inputs := range method.inputs {
		productionValue = productionValue - float64(inputs.quantity)*
			((agent.priceBelief[inputs.item].high+agent.priceBelief[inputs.item].low)/2)
	}
	//Calculate the catalyst costs and subtract
	for index, catalysts := range method.catalysts {
		productionValue = productionValue - float64(catalysts.quantity)*method.consumption[index]*
			((agent.priceBelief[catalysts.item].high+agent.priceBelief[catalysts.item].low)/2)
	}

	return productionValue
}

func getAllAverageProductionValues(agent *traderAgent) map[*productionMethod]float64 {
	pvm := make(map[*productionMethod]float64)

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
//agent - pointer to the traderAgent data set
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
				if agent.job.methods[executedIndex].consumption[catalystIndex] > rand.Float64() {
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
			askBuild.offeredAsk.quantity = 1
			askBuild.offeredAsk.item = com
			//So, given the average price on the exchange, what should we sell for?
			//This instantiation sells for the average of my price belief and the
			//exchange average.
			askBuild.offeredAsk.sellFor = (agent.priceBelief[com].high + agent.priceBelief[com].low) / 2
			//(agent.priceBelief[com].high + agent.priceBelief[com].low + com.averagePrice) / 3
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
			for _, pvSingle := range spv {
				invReqs = cQMapConcat(gatherRequirements(pvSingle), invReqs)
			}

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
		bidBuild.offeredBid.quantity = 1
		bidBuild.offeredBid.item = com
		//So, given the average price on the exchange, what should we buy at?
		//This instantiation buys at the average of my price belief and the
		//exchange average.
		bidBuild.offeredBid.buyFor = (agent.priceBelief[com].high + agent.priceBelief[com].low) / 2
		//(agent.priceBelief[com].high + agent.priceBelief[com].low + com.averagePrice) / 3
		bidSlice = append(bidSlice, bidBuild)
	}

	return bidSlice
}

//agentUpdate updates the agent's inventory, price belief and cash on hand post
//market results
//agent - pointer to the traderAgent dataset
//askSlice - pointer to the post market ask slice (carrying sold data)
//bidSlice - pointer to the post market bid slice (carrying buy data)
func agentUpdate(agent *traderAgent, askSlice *[]asks, bidSlice *[]bids) {
	//Go through all the asks and tally up the sales/remove items from inventory.
	//If not accepted, lower sales price internal estimate
	bigPercent := 0.2
	littlePercent := 0.01
	for _, askSet := range *askSlice {
		agentHigh := agent.priceBelief[askSet.offeredAsk.item].high
		agentLow := agent.priceBelief[askSet.offeredAsk.item].low
		agentAvg := (agentHigh + agentLow) / 2
		itemAvg := askSet.offeredAsk.item.averagePrice
		if askSet.numberAccepted > 0 {
			//AskSet was accepted!  Take out that much inventory and add cash.
			fmt.Printf("Ask Accepted! %v units of %v for %v\n", askSet.numberAccepted, askSet.offeredAsk.item.name, askSet.offeredAsk.sellFor)
			agent.funds = agent.funds + (float64(askSet.offeredAsk.quantity) * float64(askSet.numberAccepted) * askSet.offeredAsk.sellFor)
			agent.inventory[askSet.offeredAsk.item] = agent.inventory[askSet.offeredAsk.item] - (askSet.offeredAsk.quantity * askSet.numberAccepted)
			//Consider raising our prices - a lot if we're under the average, a little if we're over.
			if agentAvg <= itemAvg {
				//Agent Average under Average - Raise a lot!
				agentHigh = agentHigh + math.Abs(agentHigh-itemAvg)*bigPercent
				agentLow = agentLow + math.Abs(agentLow-itemAvg)*bigPercent
				//Bring it back down if too big.
				for agentLow >= agentHigh {
					agentLow = agentLow - math.Abs(agentLow-itemAvg)*bigPercent
					fmt.Println("INVERT1")
				}
			} else {
				//Overaverage!  Raise just a bit.
				agentHigh = agentHigh + math.Abs(agentHigh-itemAvg)*littlePercent
				agentLow = agentLow + math.Abs(agentLow-itemAvg)*littlePercent
				for agentLow >= agentHigh {
					agentLow = agentLow - math.Abs(agentLow-itemAvg)*littlePercent
					fmt.Println("INVERT2")
				}
			}

		} else {
			//None were accepted!  This means our price was too high. =(
			//Consider, are we larger than the average?  Lower it down towards the average by a lot.
			//Are we lower than the average?  Lower it down a little bit.
			if agentAvg >= itemAvg {
				//Agent Average over Average - Lower a lot!
				agentHigh = agentHigh - math.Abs(agentHigh-itemAvg)*bigPercent
				agentLow = agentLow - math.Abs(agentLow-itemAvg)*bigPercent
				for agentLow >= agentHigh {
					agentLow = agentLow - math.Abs(agentLow-itemAvg)*bigPercent
					fmt.Println("INVERT3")
				}
			} else {
				//Under Average
				agentHigh = agentHigh - math.Abs(agentHigh-itemAvg)*littlePercent
				agentLow = agentLow - math.Abs(agentLow-itemAvg)*littlePercent
				for agentLow >= agentHigh {
					agentLow = agentLow - math.Abs(agentLow-itemAvg)*littlePercent
					//fmt.Println("INVERT4")
				}
			}
		}
		//if agentHigh < askSet.offeredAsk.item.averagePrice {
		//	agentHigh = askSet.offeredAsk.item.averagePrice
		//}
		if agentLow < 0 {
			agentLow = 0.5 //Totally arbitrary
		}
		var agentPriceBelief = agent.priceBelief[askSet.offeredAsk.item]
		agentPriceBelief.high = agentHigh
		agentPriceBelief.low = agentLow
		//fmt.Printf("Price on %v: Low: %v, High: %v, Current Average: %v\n", askSet.offeredAsk.item.name, agentLow, agentHigh, askSet.offeredAsk.item.averagePrice)
		agent.priceBelief[askSet.offeredAsk.item] = agentPriceBelief
	}

	//Go through all the bids.
	//Clear buys, remove money, add inventory, alter prices
	for _, bidSet := range *bidSlice {
		agentHigh := agent.priceBelief[bidSet.offeredBid.item].high
		agentLow := agent.priceBelief[bidSet.offeredBid.item].low
		agentAvg := (agentHigh + agentLow) / 2
		itemAvg := bidSet.offeredBid.item.averagePrice
		if bidSet.numberAccepted > 0 {
			//bidSet was accepted!  Give inventory and remove cash
			agent.funds = agent.funds - (float64(bidSet.offeredBid.quantity) * float64(bidSet.numberAccepted) * bidSet.offeredBid.buyFor)
			agent.inventory[bidSet.offeredBid.item] = agent.inventory[bidSet.offeredBid.item] + (bidSet.offeredBid.quantity * bidSet.numberAccepted)
			//Consider lowering our prices - a lot if we're over the average, a little if we're under.
			if agentAvg >= itemAvg {
				//Agent Average over Average - Lower a lot!
				agentHigh = agentHigh - math.Abs(agentHigh-itemAvg)*bigPercent
				agentLow = agentLow - math.Abs(agentLow-itemAvg)*bigPercent
				for agentLow >= agentHigh {
					agentLow = agentLow - math.Abs(agentLow-itemAvg)*bigPercent
					fmt.Println("INVERT5")
				}
			} else {
				//Under Average
				agentHigh = agentHigh - math.Abs(agentHigh-itemAvg)*littlePercent
				agentLow = agentLow - math.Abs(agentLow-itemAvg)*littlePercent
				for agentLow >= agentHigh {
					agentLow = agentLow - math.Abs(agentLow-itemAvg)*littlePercent
					fmt.Println("INVERT6")
				}
			}

		} else {
			//None were accepted!  This means our price was too low. =(
			//Consider, are we larger than the average?  Raise it down towards the average by a little.
			//Are we lower than the average?  Raise it a lot
			if agentAvg <= itemAvg {
				//Agent Average under Average - Raise a lot!
				agentHigh = agentHigh + math.Abs(agentHigh-itemAvg)*bigPercent
				agentLow = agentLow + math.Abs(agentLow-itemAvg)*bigPercent
				//Bring it back down if too big.
				for agentLow >= agentHigh {
					agentLow = agentLow - math.Abs(agentLow-itemAvg)*bigPercent
					fmt.Println("INVERT7")
				}
			} else {
				//Overaverage!  Raise just a bit.
				agentHigh = agentHigh + math.Abs(agentHigh-itemAvg)*littlePercent
				agentLow = agentLow + math.Abs(agentLow-itemAvg)*littlePercent
				for agentLow >= agentHigh {
					agentLow = agentLow - math.Abs(agentLow-itemAvg)*littlePercent
					fmt.Println("INVERT8")
				}
			}
		}
		//if agentHigh < bidSet.offeredBid.item.averagePrice {
		//	agentHigh = bidSet.offeredBid.item.averagePrice
		//}
		if agentLow < 0 {
			agentLow = 0.5 //Totally arbitrary
		}
		var agentPriceBelief = agent.priceBelief[bidSet.offeredBid.item]
		agentPriceBelief.high = agentHigh
		agentPriceBelief.low = agentLow
		agent.priceBelief[bidSet.offeredBid.item] = agentPriceBelief
	}
}

//Generates an initial random price belief for an agent.  It is set to high >
//averagePrice and low > 0
//commoditySlice - a slice of commodity pointers
//Returns a map of commodity pointers to price range
func randomPriceBelief(commodityList map[string]*commodity) map[*commodity]priceRange {
	prMap := make(map[*commodity]priceRange)
	for _, aCommodity := range commodityList {
		var pr priceRange
		pr.high = aCommodity.averagePrice + (rand.Float64() * aCommodity.averagePrice)
		pr.low = aCommodity.averagePrice - (rand.Float64() * aCommodity.averagePrice)
		prMap[aCommodity] = pr
	}
	return prMap
}

func main() {
	fmt.Println("Economic Simulation")
	fmt.Println("Set up our commodities")
	var wood commodity
	wood.name = "Wood"
	wood.averagePrice = 3
	var tools commodity
	tools.name = "Tools"
	tools.averagePrice = 3
	var food commodity
	food.name = "Food"
	food.averagePrice = 3
	var ore commodity
	ore.name = "Ore"
	ore.averagePrice = 3
	var metal commodity
	metal.name = "Metal"
	metal.averagePrice = 3

	allCommodities := make(map[string]*commodity)
	allCommodities["Wood"] = &wood
	allCommodities["Tools"] = &tools
	allCommodities["Food"] = &food
	allCommodities["Ore"] = &ore
	allCommodities["Metal"] = &metal

	//Commodity Sets
	//Food
	var singleFood commoditySet
	singleFood.item = &food
	singleFood.quantity = 1
	var twoFood commoditySet
	twoFood.item = &food
	twoFood.quantity = 2
	var fourFood commoditySet
	fourFood.item = &food
	fourFood.quantity = 4
	//Wood
	var singleWood commoditySet
	singleWood.item = &wood
	singleWood.quantity = 1
	var twoWood commoditySet
	twoWood.item = &wood
	twoWood.quantity = 2
	var fourWood commoditySet
	fourWood.item = &wood
	fourWood.quantity = 4
	//Ore
	var twoOre commoditySet
	twoOre.item = &ore
	twoOre.quantity = 2
	var fourOre commoditySet
	fourOre.item = &ore
	fourOre.quantity = 4
	//Metal
	var twoMetal commoditySet
	twoMetal.item = &metal
	twoMetal.quantity = 2
	var fourMetal commoditySet
	fourMetal.item = &metal
	fourMetal.quantity = 4
	//Tools
	var singleTools commoditySet
	singleTools.item = &tools
	singleTools.quantity = 1
	var twoTools commoditySet
	twoTools.item = &tools
	twoTools.quantity = 2
	var fourTools commoditySet
	fourTools.item = &tools
	fourTools.quantity = 4

	fmt.Println("Set up our production rules")
	//Farmer
	var farmerProd productionMethod
	farmerProd.inputs = append(farmerProd.inputs, singleWood)
	farmerProd.outputs = append(farmerProd.outputs, twoFood)
	var farmerToolsProd productionMethod
	farmerToolsProd.inputs = farmerProd.inputs
	farmerToolsProd.outputs = append(farmerToolsProd.outputs, fourFood)
	farmerToolsProd.catalysts = append(farmerToolsProd.catalysts, singleTools)
	farmerToolsProd.consumption = append(farmerToolsProd.consumption, 0.1)
	var farmerProdSet productionSet
	farmerProdSet.methods = make([]*productionMethod, 2)
	farmerProdSet.methods[0] = &farmerProd
	farmerProdSet.methods[1] = &farmerToolsProd
	farmerProdSet.penalty = 2
	//Miner
	var minerProd productionMethod
	minerProd.inputs = append(minerProd.inputs, singleFood)
	minerProd.outputs = append(minerProd.outputs, twoOre)
	var minerToolsProd productionMethod
	minerToolsProd.inputs = minerProd.inputs
	minerToolsProd.outputs = append(minerToolsProd.outputs, fourOre)
	minerToolsProd.catalysts = append(minerToolsProd.catalysts, singleTools)
	minerToolsProd.consumption = append(minerToolsProd.consumption, 0.1)
	var minerProdSet productionSet
	minerProdSet.methods = make([]*productionMethod, 2)
	minerProdSet.methods[0] = &minerProd
	minerProdSet.methods[1] = &minerToolsProd
	minerProdSet.penalty = 2
	//Refiner
	var refinerProd productionMethod
	refinerProd.inputs = make([]commoditySet, 2)
	refinerProd.inputs[0] = singleFood
	refinerProd.inputs[1] = twoOre
	refinerProd.outputs = append(refinerProd.outputs, twoMetal)
	var refinerToolsProd productionMethod
	refinerToolsProd.inputs = make([]commoditySet, 2)
	refinerToolsProd.inputs[0] = singleFood
	refinerToolsProd.inputs[1] = fourOre
	refinerToolsProd.outputs = append(refinerToolsProd.outputs, fourMetal)
	refinerToolsProd.catalysts = append(refinerToolsProd.catalysts, singleTools)
	refinerToolsProd.consumption = append(refinerToolsProd.consumption, 0.1)
	var refinerProdSet productionSet
	refinerProdSet.methods = make([]*productionMethod, 2)
	refinerProdSet.methods[0] = &refinerProd
	refinerProdSet.methods[1] = &refinerToolsProd
	refinerProdSet.penalty = 2
	//Woodcutter
	var woodcutterProd productionMethod
	woodcutterProd.inputs = append(woodcutterProd.inputs, singleFood)
	woodcutterProd.outputs = append(woodcutterProd.outputs, singleWood)
	var woodcutterToolsProd productionMethod
	woodcutterToolsProd.inputs = woodcutterProd.inputs
	woodcutterToolsProd.outputs = append(woodcutterToolsProd.outputs, twoWood)
	woodcutterToolsProd.catalysts = append(woodcutterToolsProd.catalysts, singleTools)
	woodcutterToolsProd.consumption = append(woodcutterToolsProd.consumption, 0.1)
	var woodcutterProdSet productionSet
	woodcutterProdSet.methods = make([]*productionMethod, 2)
	woodcutterProdSet.methods[0] = &woodcutterProd
	woodcutterProdSet.methods[1] = &woodcutterToolsProd
	woodcutterProdSet.penalty = 2
	//Blacksmith
	var blacksmithProd productionMethod
	blacksmithProd.inputs = make([]commoditySet, 2)
	blacksmithProd.inputs[0] = singleFood
	blacksmithProd.inputs[1] = twoMetal
	blacksmithProd.outputs = append(blacksmithProd.outputs, twoTools)
	var blacksmithDoubleProd productionMethod
	blacksmithDoubleProd.inputs = make([]commoditySet, 2)
	blacksmithDoubleProd.inputs[0] = singleFood
	blacksmithDoubleProd.inputs[1] = fourMetal
	blacksmithDoubleProd.outputs = append(blacksmithDoubleProd.outputs, fourTools)
	var blacksmithProdSet productionSet
	blacksmithProdSet.methods = make([]*productionMethod, 2)
	blacksmithProdSet.methods[0] = &blacksmithProd
	blacksmithProdSet.methods[1] = &blacksmithDoubleProd
	blacksmithProdSet.penalty = 2

	fmt.Println("Set up our traders!")
	////makeFarmer Example
	//farmer := makeFarmer(allCommodities, &farmerProdSet)
	////makeMiner Example
	//miner := makeMiner(allCommodities, &minerProdSet)
	////makeRefiner Example
	//refiner := makeRefiner(allCommodities, &refinerProdSet)
	////makeWoodcutter Example
	//woodcutter := makeWoodcutter(allCommodities, &woodcutterProdSet)
	////makeBlacksmith Example
	//blacksmith := makeBlacksmith(allCommodities, &blacksmithProdSet)

	//Set the cohort sizes
	numFarmers := 500
	numMiners := 500
	numRefiners := 500
	numWoodcutters := 500
	numBlacksmiths := 500
	totalTraders := numFarmers + numMiners + numRefiners + numWoodcutters + numBlacksmiths
	askChannels := make([]chan []asks, totalTraders)
	bidChannels := make([]chan []bids, totalTraders)
	deadChannels := make([]chan traderAgent, totalTraders)
	tempAskChannel := make(chan []asks)
	tempBidChannel := make(chan []bids)
	tempDeadChannel := make(chan traderAgent)
	for i := 0; i < numFarmers; i++ {
		tempAskChannel, tempBidChannel, tempDeadChannel = agentRun(makeFarmer(allCommodities, &farmerProdSet))
		askChannels = append(askChannels, tempAskChannel)
		bidChannels = append(bidChannels, tempBidChannel)
		deadChannels = append(deadChannels, tempDeadChannel)
	}
	for i := 0; i < numMiners; i++ {
		tempAskChannel, tempBidChannel, tempDeadChannel = agentRun(makeMiner(allCommodities, &minerProdSet))
		askChannels = append(askChannels, tempAskChannel)
		bidChannels = append(bidChannels, tempBidChannel)
		deadChannels = append(deadChannels, tempDeadChannel)
	}
	for i := 0; i < numRefiners; i++ {
		tempAskChannel, tempBidChannel, tempDeadChannel = agentRun(makeRefiner(allCommodities, &refinerProdSet))
		askChannels = append(askChannels, tempAskChannel)
		bidChannels = append(bidChannels, tempBidChannel)
		deadChannels = append(deadChannels, tempDeadChannel)
	}
	for i := 0; i < numWoodcutters; i++ {
		tempAskChannel, tempBidChannel, tempDeadChannel = agentRun(makeWoodcutter(allCommodities, &woodcutterProdSet))
		askChannels = append(askChannels, tempAskChannel)
		bidChannels = append(bidChannels, tempBidChannel)
		deadChannels = append(deadChannels, tempDeadChannel)
	}
	for i := 0; i < numBlacksmiths; i++ {
		tempAskChannel, tempBidChannel, tempDeadChannel = agentRun(makeBlacksmith(allCommodities, &blacksmithProdSet))
		askChannels = append(askChannels, tempAskChannel)
		bidChannels = append(bidChannels, tempBidChannel)
		deadChannels = append(deadChannels, tempDeadChannel)
	}

	fmt.Println("Set up a market!")
	//Make the ask and bid books
	//Break them by type
	asksTyped := make(map[*commodity][]*asks)
	bidsTyped := make(map[*commodity][]*bids)
	for _, com := range allCommodities {
		var asksBlank []*asks
		var bidsBlank []*bids
		asksTyped[com] = asksBlank
		bidsTyped[com] = bidsBlank
	}
	//totalTimeMillis := 300
	ticker := time.NewTicker(time.Millisecond * 500)
	go func() {
		for t := range ticker.C {
			fmt.Println("tick at", t)
			//RECEIVE ALL THE ASKS AND BIDS

			//Check all the ask channels
			var tempAsksStorage []asks
			for com, _ := range asksTyped {
				asksTyped[com] = nil
			}
			for chindex, channel := range askChannels {
				select {
				case tempAsksStorage = <-channel:
					//fmt.Println("Got an *[]asks on ", chindex)
					for _, asksIn := range tempAsksStorage {
						//Add them to the ask book
						asksIn.offeredAsk.id = uint64(chindex)
						asksTyped[asksIn.offeredAsk.item] = append(asksTyped[asksIn.offeredAsk.item], &asksIn)
					}
				default:
					//fmt.Println("No Asks on %v", chindex)
				}
			}
			var tempBidsStorage []bids
			for com, _ := range bidsTyped {
				bidsTyped[com] = nil
			}
			for chindex, channel := range bidChannels {
				select {
				case tempBidsStorage = <-channel:
					//fmt.Println("Got a *[]bids on %v", chindex)
					for _, bidsIn := range tempBidsStorage {
						//Add them to the bids book
						bidsIn.offeredBid.id = uint64(chindex)
						bidsTyped[bidsIn.offeredBid.item] = append(bidsTyped[bidsIn.offeredBid.item], &bidsIn)
					}
				default:
					//fmt.Println("No Bid on %v", chindex)
				}
			}

			fmt.Println("Total Asks Types: ", len(asksTyped))
			fmt.Println("Total Bids Types: ", len(bidsTyped))

			//Sort the Asks and Bids within each type
			for com, asksCom := range asksTyped {
				fmt.Printf("Asks for %v: %v\n", com.name, len(asksCom))
				sort.Sort(AsksLowToHigh(asksCom))
			}
			for com, bidsCom := range bidsTyped {
				fmt.Printf("Bids for %v: %v\n", com.name, len(bidsCom))
				sort.Sort(BidsHighToLow(bidsCom))
			}

			for com, asksCom := range asksTyped {
				//Comparison: Lowest Ask to Highest Bid
				bidsCom := bidsTyped[com]
				//continue to match them, executing clearing trades as we go.
				asksIndex := 0
				bidsIndex := 0
				totalTransactions := 0
				var runningTotal float64
				runningTotal = 0.0
				if len(asksCom) > 0 && len(bidsCom) > 0 {
					for {
						asksQuantityRemaining := asksCom[asksIndex].numberOffered - asksCom[asksIndex].numberAccepted
						bidsQuantityRemaining := bidsCom[bidsIndex].numberOffered - bidsCom[bidsIndex].numberAccepted
						//Make sure prices are still acceptable - are there bids greater than asks in existance?
						if asksCom[asksIndex].offeredAsk.sellFor > bidsCom[bidsIndex].offeredBid.buyFor {
							break
						}
						//We're in business then - keep rollin'.
						if asksQuantityRemaining >= bidsQuantityRemaining {
							asksCom[asksIndex].numberAccepted += bidsQuantityRemaining
							bidsCom[bidsIndex].numberAccepted = bidsCom[bidsIndex].numberOffered
							totalTransactions += bidsCom[bidsIndex].numberAccepted
							if asksQuantityRemaining != bidsQuantityRemaining {
								//Split to add a new ask with the remaining bit (since we need to communicate back our price)
								tempAsksComPre := asksCom[:asksIndex+1]  //Get everything before including our current index
								tempAsksComPost := asksCom[asksIndex+1:] //Get everything after our current index
								newAsk := asksCom[asksIndex].offeredAsk
								newAsks := asksCom[asksIndex]
								newAsks.numberAccepted = 0
								newAsks.numberOffered = asksCom[asksIndex].numberOffered - asksCom[asksIndex].numberAccepted
								newAsks.offeredAsk = newAsk
								asksCom = append(tempAsksComPre, newAsks)
								asksCom = append(asksCom, tempAsksComPost...)
							}
							//OK! New one added, let's clear the rest of it.
							asksCom[asksIndex].numberOffered = asksCom[asksIndex].numberAccepted
							asksCom[asksIndex].offeredAsk.sellFor = (asksCom[asksIndex].offeredAsk.sellFor + bidsCom[bidsIndex].offeredBid.buyFor) / 2.0
							bidsCom[bidsIndex].offeredBid.buyFor = asksCom[asksIndex].offeredAsk.sellFor
							runningTotal += bidsCom[bidsIndex].offeredBid.buyFor * float64(bidsCom[bidsIndex].numberAccepted)
						} else {
							//OK, more bids than asks instead.
							bidsCom[bidsIndex].numberAccepted += asksQuantityRemaining
							asksCom[asksIndex].numberAccepted = asksCom[asksIndex].numberOffered
							totalTransactions += asksCom[asksIndex].numberAccepted
							//Split to add a new bid with the remaining bit (since we need to communicate back our price)
							tempBidsComPre := bidsCom[:bidsIndex+1]  //Get everything before including our current index
							tempBidsComPost := bidsCom[bidsIndex+1:] //Get everything after our current index
							newBid := bidsCom[bidsIndex].offeredBid
							newBids := bidsCom[bidsIndex]
							newBids.numberAccepted = 0
							newBids.numberOffered = bidsCom[bidsIndex].numberOffered - bidsCom[bidsIndex].numberAccepted
							newBids.offeredBid = newBid
							bidsCom = append(tempBidsComPre, newBids)
							bidsCom = append(bidsCom, tempBidsComPost...)
							//OK! new one added, let's clear the rest of it.
							bidsCom[bidsIndex].numberOffered = bidsCom[bidsIndex].numberAccepted
							asksCom[asksIndex].offeredAsk.sellFor = (asksCom[asksIndex].offeredAsk.sellFor + bidsCom[bidsIndex].offeredBid.buyFor) / 2.0
							bidsCom[bidsIndex].offeredBid.buyFor = asksCom[asksIndex].offeredAsk.sellFor
							runningTotal += asksCom[asksIndex].offeredAsk.sellFor * float64(asksCom[asksIndex].numberAccepted)
						}
						//increase the indexes
						bidsIndex++
						asksIndex++
						//fmt.Printf("AskIndex: %v , BidIndex: %v\n", asksIndex, bidsIndex)

						//while both bids and asks have remaining individuals
						if bidsIndex >= len(bidsCom) || asksIndex >= len(asksCom) {
							break
						}
					}
				}
				if totalTransactions != 0 {
					com.averagePrice = runningTotal / float64(totalTransactions)
				} else {
					fmt.Printf("No transactions of %v!\n", com.name)
				}
			}

			//OK! Market Cleared.  Communicate results
			fmt.Println("Market Cleared!")
			for index, askChannel := range askChannels {
				var asksOut []asks
				//Search the results for matching results to send on the channel
				for _, asksCom := range asksTyped {
					for _, asksTest := range asksCom {
						if asksTest.offeredAsk.id == uint64(index) {
							asksOut = append(asksOut, *asksTest)
						}
					}
				}
				select {
				case askChannel <- asksOut:
					//fmt.Println("Sent a message!")
				default:
				}
			}
			fmt.Println("Done sending over askChannels")

			for index, bidChannel := range bidChannels {
				var bidsOut []bids
				//Search the results for matching results to send on the channel
				for _, bidsCom := range bidsTyped {
					for _, bidsTest := range bidsCom {
						if bidsTest.offeredBid.id == uint64(index) {
							bidsOut = append(bidsOut, *bidsTest)
						}
					}
				}
				select {
				case bidChannel <- bidsOut:
					//fmt.Println("Sent a Bid Message")
				default:
				}
			}

			//Check for Deads and Regen
			for chindex, channel := range deadChannels {
				var deadAgent traderAgent
				select {
				case deadAgent = <-channel:
					fmt.Println("Got a dead on ", chindex)
					switch deadAgent.role {
					case "Farmer":
						numFarmers--
					case "Miner":
						numMiners--
					case "Refiner":
						numRefiners--
					case "Woodcutter":
						numWoodcutters--
					case "Blacksmith":
						numBlacksmiths--
					}

					//Which Commodity is the most expensive?
					maxCom := allCommodities["Food"]
					for _, com := range allCommodities {
						if com.averagePrice > maxCom.averagePrice {
							maxCom = com
						}
					}

					//Make that one!
					switch maxCom.name {
					case "Food":
						askChannels[chindex], bidChannels[chindex], deadChannels[chindex] = agentRun(makeFarmer(allCommodities, &farmerProdSet))
						numFarmers++
					case "Ore":
						askChannels[chindex], bidChannels[chindex], deadChannels[chindex] = agentRun(makeMiner(allCommodities, &minerProdSet))
						numMiners++
					case "Metal":
						askChannels[chindex], bidChannels[chindex], deadChannels[chindex] = agentRun(makeRefiner(allCommodities, &refinerProdSet))
						numRefiners++
					case "Wood":
						askChannels[chindex], bidChannels[chindex], deadChannels[chindex] = agentRun(makeWoodcutter(allCommodities, &woodcutterProdSet))
						numWoodcutters++
					case "Tools":
						askChannels[chindex], bidChannels[chindex], deadChannels[chindex] = agentRun(makeBlacksmith(allCommodities, &blacksmithProdSet))
						numBlacksmiths++
					}

				default:
					//fmt.Println("No Deads on %v", chindex)
				}
			}
			//Output our live counts!
			fmt.Println("\nAgent Count!")
			fmt.Println("Farmers: ", numFarmers)
			fmt.Println("Miners: ", numMiners)
			fmt.Println("Refiners: ", numRefiners)
			fmt.Println("Woodcutters: ", numWoodcutters)
			fmt.Println("Blacksmiths: ", numBlacksmiths)

			fmt.Println("\nPrices!")
			fmt.Println("Food: ", allCommodities["Food"].averagePrice)
			fmt.Println("Ore: ", allCommodities["Ore"].averagePrice)
			fmt.Println("Metal: ", allCommodities["Metal"].averagePrice)
			fmt.Println("Wood: ", allCommodities["Wood"].averagePrice)
			fmt.Println("Tools: ", allCommodities["Tools"].averagePrice)
		}
	}()

	//Block forever
	select {}
}

//This is the definition of the sort asks lowest to highest
type AsksLowToHigh []*asks

func (a AsksLowToHigh) Len() int           { return len(a) }
func (a AsksLowToHigh) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a AsksLowToHigh) Less(i, j int) bool { return a[i].offeredAsk.sellFor < a[j].offeredAsk.sellFor }

//This is the definition of the sort bids from highest to lowest
type BidsHighToLow []*bids

func (a BidsHighToLow) Len() int           { return len(a) }
func (a BidsHighToLow) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a BidsHighToLow) Less(i, j int) bool { return a[i].offeredBid.buyFor > a[j].offeredBid.buyFor } //THIS MAY NOT WORK

func makeFarmer(commodityList map[string]*commodity, prodSet *productionSet) traderAgent {
	var farmerOut traderAgent
	farmerOut.role = "Farmer"
	farmerOut.funds = 50 + (rand.Float64() * 50)
	farmerOut.inventory = make(map[*commodity]int)
	if grantGoods {
		farmerOut.inventory[commodityList["Tools"]] = rand.Intn(2)
		farmerOut.inventory[commodityList["Wood"]] = rand.Intn(4) + 2
	}
	farmerOut.job = prodSet
	farmerOut.priceBelief = randomPriceBelief(commodityList)
	farmerOut.riskAversion = rand.Intn(4) + 1
	return farmerOut
}

func makeMiner(commodityList map[string]*commodity, prodSet *productionSet) traderAgent {
	var minerOut traderAgent
	minerOut.role = "Miner"
	minerOut.funds = 50 + (rand.Float64() * 50)
	minerOut.inventory = make(map[*commodity]int)
	if grantGoods {
		minerOut.inventory[commodityList["Tools"]] = rand.Intn(2)
		minerOut.inventory[commodityList["Food"]] = rand.Intn(4) + 2
	}
	minerOut.job = prodSet
	minerOut.priceBelief = randomPriceBelief(commodityList)
	minerOut.riskAversion = rand.Intn(4) + 1
	return minerOut
}

func makeRefiner(commodityList map[string]*commodity, prodSet *productionSet) traderAgent {
	var refinerOut traderAgent
	refinerOut.role = "Refiner"
	refinerOut.funds = 50 + (rand.Float64() * 50)
	refinerOut.inventory = make(map[*commodity]int)
	if grantGoods {
		refinerOut.inventory[commodityList["Ore"]] = 2 + rand.Intn(3)
		refinerOut.inventory[commodityList["Food"]] = rand.Intn(4) + 2
		refinerOut.inventory[commodityList["Tools"]] = rand.Intn(2)
	}
	refinerOut.job = prodSet
	refinerOut.priceBelief = randomPriceBelief(commodityList)
	refinerOut.riskAversion = rand.Intn(4) + 1
	return refinerOut
}

func makeWoodcutter(commodityList map[string]*commodity, prodSet *productionSet) traderAgent {
	var woodcutterOut traderAgent
	woodcutterOut.role = "Woodcutter"
	woodcutterOut.funds = 50 + (rand.Float64() * 50)
	woodcutterOut.inventory = make(map[*commodity]int)
	if grantGoods {
		woodcutterOut.inventory[commodityList["Tools"]] = rand.Intn(2)
		woodcutterOut.inventory[commodityList["Food"]] = rand.Intn(4) + 2
	}
	woodcutterOut.job = prodSet
	woodcutterOut.priceBelief = randomPriceBelief(commodityList)
	woodcutterOut.riskAversion = rand.Intn(4) + 1
	return woodcutterOut
}

func makeBlacksmith(commodityList map[string]*commodity, prodSet *productionSet) traderAgent {
	var blacksmithOut traderAgent
	blacksmithOut.role = "Blacksmith"
	blacksmithOut.funds = 50 + (rand.Float64() * 50)
	blacksmithOut.inventory = make(map[*commodity]int)
	if grantGoods {
		blacksmithOut.inventory[commodityList["Metal"]] = 2 + rand.Intn(3)
		blacksmithOut.inventory[commodityList["Food"]] = rand.Intn(4) + 2
	}
	blacksmithOut.job = prodSet
	blacksmithOut.priceBelief = randomPriceBelief(commodityList)
	blacksmithOut.riskAversion = rand.Intn(4) + 1
	return blacksmithOut
}

//Set up our agent system/world state in here.
func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	fmt.Printf("Number of CPUS: %d\n", runtime.NumCPU())
	rand.Seed(time.Now().UTC().UnixNano())
	//Flags!
	grantGoods = true
}
