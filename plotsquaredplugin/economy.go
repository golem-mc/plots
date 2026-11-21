package plotsquaredplugin

import (
	"fmt"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plotsquared"
	"github.com/golem-mc/plotsquared/event"
	"github.com/golem-mc/vault"
	"github.com/google/uuid"
)

// Economy returns the registered Vault Economy provider if economy is enabled in settings.yml.
func (ps *PlotSquared) Economy() (vault.Economy, bool) {
	if ps.Settings != nil && !ps.Settings.EnabledComponents.Economy {
		return nil, false
	}
	return plugin.Service[vault.Economy](ps.Server)
}

// ChargeAreaPrice withdraws an area economy price (claim/merge) from player if economy is active.
// Returns (cost, true, nil) on success, or (cost, false, err) if the player cannot afford it.
func (ps *PlotSquared) ChargeAreaPrice(p plugin.Player, area *plotsquared.PlotArea, expr string) (float64, bool, error) {
	if p == nil || area == nil || !area.UseEconomy || HasPerm(p, "plots.eco.bypass") {
		return 0, true, nil
	}
	eco, ok := ps.Economy()
	if !ok {
		return 0, true, nil
	}
	owned := len(ps.PlotsByOwner(p.UUID()))
	cost := plotsquared.EvalPrice(expr, owned)
	if cost <= 0 {
		return 0, true, nil
	}
	if !eco.Has(p.UUID(), cost) {
		return cost, false, fmt.Errorf("insufficient funds: need %s", eco.Format(cost))
	}
	resp := eco.Withdraw(p.UUID(), cost)
	if !resp.Success() {
		return cost, false, resp.Err
	}
	return cost, true, nil
}

// RefundAreaPrice deposits money back to a player (e.g. on plot delete/sell).
func (ps *PlotSquared) RefundAreaPrice(u uuid.UUID, area *plotsquared.PlotArea, expr string) float64 {
	if u == uuid.Nil || area == nil || !area.UseEconomy {
		return 0
	}
	eco, ok := ps.Economy()
	if !ok {
		return 0
	}
	amount := plotsquared.EvalPrice(expr, len(ps.PlotsByOwner(u)))
	if amount <= 0 {
		return 0
	}
	eco.Deposit(u, amount)
	return amount
}

// BuyPlot transfers a plot that has a positive `price` flag from its current owner to buyer via Vault.
func (ps *PlotSquared) BuyPlot(buyer plugin.Player, plot *plotsquared.Plot) (float64, error) {
	if buyer == nil || plot == nil || !plot.HasOwner() {
		return 0, fmt.Errorf("plot is not for sale")
	}
	base := plot.BasePlot()
	if base.IsOwner(buyer.UUID()) {
		return 0, fmt.Errorf("you already own this plot")
	}
	price, _ := base.FlagValue("price").(float64)
	if price <= 0 {
		return 0, fmt.Errorf("plot is not for sale")
	}
	eco, ok := ps.Economy()
	if !ok {
		return 0, fmt.Errorf("economy is not enabled")
	}
	if !eco.Has(buyer.UUID(), price) {
		return 0, fmt.Errorf("you need %s to buy this plot", eco.Format(price))
	}

	ev := &event.PlayerBuyPlotEvent{Player: buyer, Plot: base, Price: price}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		return 0, fmt.Errorf("purchase cancelled")
	}

	resp := eco.Withdraw(buyer.UUID(), price)
	if !resp.Success() {
		return 0, resp.Err
	}
	oldOwner := base.Owner
	if oldOwner != uuid.Nil {
		eco.Deposit(oldOwner, price)
	}
	if err := ps.SetOwner(base, buyer.UUID(), buyer); err != nil {
		return 0, err
	}
	_, _ = ps.RemoveFlag(base, "price")
	return price, nil
}
