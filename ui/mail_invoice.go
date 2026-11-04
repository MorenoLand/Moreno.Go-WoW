package ui

import (
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// What an auction mail is about, by the last number of its subject.
const (
	auctionMailWon      = 1
	auctionMailSold     = 2
	auctionMailTempSale = 6
)

// auctionInvoice is the account of an auction a mail carries: its subject names
// the item and what happened, its text the money.
type auctionInvoice struct {
	kind                           string
	item                           string
	player                         string
	bid, buyout, deposit, cut      int64
	moneyDelay, etaHours, etaMinut int64
}

// invoiceOf reads the invoice of an auction mail.
func (rt *Runtime) invoiceOf(mail *world.MailMessage) (auctionInvoice, bool) {
	parts := strings.Split(mail.Subject, ":")
	if len(parts) != 3 {
		return auctionInvoice{}, false
	}
	entry, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil || entry == 0 {
		return auctionInvoice{}, false
	}
	action, err := strconv.Atoi(parts[2])
	if err != nil {
		return auctionInvoice{}, false
	}
	invoice := auctionInvoice{kind: "buyer"}
	switch action {
	case auctionMailSold:
		invoice.kind = "seller"
	case auctionMailTempSale:
		invoice.kind = "seller_temp_invoice"
	}
	if template := rt.itemTemplate(uint32(entry)); template != nil {
		invoice.item = template.Name
	}
	fields := strings.Split(mail.Body, ":")
	want := map[int]int{auctionMailWon: 3, auctionMailSold: 5, auctionMailTempSale: 7}[action]
	if want == 0 || len(fields) < want {
		return invoice, true
	}
	guid, err := strconv.ParseUint(fields[0], 16, 64)
	if err != nil {
		return invoice, true
	}
	invoice.player = rt.nameOf(guid)
	numbers := make([]int64, want-1)
	for index := range numbers {
		numbers[index], _ = strconv.ParseInt(fields[index+1], 10, 64)
	}
	invoice.bid, invoice.buyout = numbers[0], numbers[1]
	if want >= 5 {
		invoice.deposit, invoice.cut = numbers[2], numbers[3]
	}
	if want == 7 {
		invoice.moneyDelay = numbers[4]
		seconds := numbers[5]
		invoice.etaHours, invoice.etaMinut = seconds/3600, seconds%3600/60
	}
	return invoice, true
}

func registerMailInvoiceAPI(rt *Runtime) {
	rt.L.SetGlobal("GetInboxInvoiceInfo", rt.L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetInboxInvoiceInfo(index)")
			return 0
		}
		if mail := rt.mailAt(L.CheckInt(1)); mail != nil {
			if invoice, ok := rt.invoiceOf(mail); ok {
				L.Push(lua.LString(invoice.kind))
				L.Push(lua.LString(invoice.item))
				L.Push(lua.LString(invoice.player))
				for _, number := range []int64{invoice.bid, invoice.buyout, invoice.deposit, invoice.cut, invoice.moneyDelay, invoice.etaHours, invoice.etaMinut} {
					L.Push(lua.LNumber(number))
				}
				return 10
			}
		}
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		for i := 0; i < 4; i++ {
			L.Push(lua.LNumber(0))
		}
		return 7
	}))
}
