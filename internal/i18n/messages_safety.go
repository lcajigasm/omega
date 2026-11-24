package i18n

// Messages of the agent and its safety policy added in the fifth review.
// Same order as messages: en, es, fr, de, pt.
var safetyMessages = map[string][]string{
	"agent.cost_unknown": {
		"omega does not know the price of %s: --max-cost $%.2f counts it at a cautious estimate and may stop early",
		"omega no sabe el precio de %s: --max-cost %.2f $ lo cuenta con una estimación prudente y puede parar antes",
		"omega ne connaît pas le prix de %s : --max-cost %.2f $ le compte avec une estimation prudente et peut s'arrêter plus tôt",
		"omega kennt den Preis von %s nicht: --max-cost %.2f $ rechnet vorsichtig geschätzt und kann früher stoppen",
		"o omega não sabe o preço de %s: --max-cost %.2f $ conta-o com uma estimativa prudente e pode parar antes",
	},
	"agent.cost_limit": {
		"Reached cost budget limit of $%.2f",
		"Se alcanzó el límite de coste de %.2f $",
		"Limite de coût atteinte : %.2f $",
		"Kostenlimit von %.2f $ erreicht",
		"Atingido o limite de custo de %.2f $",
	},
}

func init() {
	for k, v := range safetyMessages {
		messages[k] = v
	}
}
