package billing

import (
	"github.com/vaughan0/go-ini"
)

// Initiailze the billing system from a config file
func Init(conf ini.File) {
	invoiceInit(conf)
}
