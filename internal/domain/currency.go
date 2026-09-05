package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// CurrencyCatalogVersion identifies the ISO 4217 source used for selectable currencies.
const CurrencyCatalogVersion = "ISO 4217 List One 2026-01-01"

type CurrencyDefinition struct {
	Code       string
	MinorUnits int
	Selectable bool
}

// Source: SIX, ISO 4217 Maintenance Agency, List One published 2026-01-01.
// Fund codes, precious metals, testing codes, and non-currency accounting units are excluded.
var currentCurrencyCodes = [...]string{
	"AED", "AFN", "ALL", "AMD", "AOA", "ARS", "AUD", "AWG", "AZN",
	"BAM", "BBD", "BDT", "BHD", "BIF", "BMD", "BND", "BOB", "BRL", "BSD", "BTN", "BWP", "BYN", "BZD",
	"CAD", "CDF", "CHF", "CLP", "CNY", "COP", "CRC", "CUP", "CVE", "CZK",
	"DJF", "DKK", "DOP", "DZD", "EGP", "ERN", "ETB", "EUR",
	"FJD", "FKP", "GBP", "GEL", "GHS", "GIP", "GMD", "GNF", "GTQ", "GYD",
	"HKD", "HNL", "HTG", "HUF", "IDR", "ILS", "INR", "IQD", "IRR", "ISK",
	"JMD", "JOD", "JPY", "KES", "KGS", "KHR", "KMF", "KPW", "KRW", "KWD", "KYD", "KZT",
	"LAK", "LBP", "LKR", "LRD", "LSL", "LYD", "MAD", "MDL", "MGA", "MKD", "MMK", "MNT", "MOP", "MRU", "MUR", "MVR", "MWK", "MXN", "MYR", "MZN",
	"NAD", "NGN", "NIO", "NOK", "NPR", "NZD", "OMR", "PAB", "PEN", "PGK", "PHP", "PKR", "PLN", "PYG",
	"QAR", "RON", "RSD", "RUB", "RWF", "SAR", "SBD", "SCR", "SDG", "SEK", "SGD", "SHP", "SLE", "SOS", "SRD", "SSP", "STN", "SVC", "SYP", "SZL",
	"THB", "TJS", "TMT", "TND", "TOP", "TRY", "TTD", "TWD", "TZS", "UAH", "UGX", "USD", "UYU", "UYW", "UZS",
	"VED", "VES", "VND", "VUV", "WST", "XAF", "XCD", "XCG", "XOF", "XPF", "YER", "ZAR", "ZMW", "ZWG",
}

// Retired codes that may exist in persisted history stay here with Selectable=false.
var historicalCurrencyCodes = [...]string{}

var currencyCatalog, currencyExponents, selectableCurrencies = buildCurrencyCatalog()

func buildCurrencyCatalog() ([]CurrencyDefinition, map[string]int, map[string]bool) {
	catalog := make([]CurrencyDefinition, 0, len(currentCurrencyCodes)+len(historicalCurrencyCodes))
	exponents := make(map[string]int, cap(catalog))
	selectable := make(map[string]bool, len(currentCurrencyCodes))
	add := func(code string, enabled bool) {
		if len(code) != 3 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' || code[2] < 'A' || code[2] > 'Z' {
			panic(fmt.Sprintf("currency catalog contains malformed ISO 4217 code %q", code))
		}
		if _, exists := exponents[code]; exists {
			panic(fmt.Sprintf("currency catalog contains duplicate ISO 4217 code %q", code))
		}
		minorUnits := currencyMinorUnits(code)
		catalog = append(catalog, CurrencyDefinition{Code: code, MinorUnits: minorUnits, Selectable: enabled})
		exponents[code] = minorUnits
		selectable[code] = enabled
	}
	for _, code := range currentCurrencyCodes {
		add(code, true)
	}
	for _, code := range historicalCurrencyCodes {
		add(code, false)
	}
	return catalog, exponents, selectable
}

// currencyMinorUnits is the CcyMnrUnts value from the same SIX List One snapshot.
// Most currencies use two decimal places, so only official exceptions are listed.
func currencyMinorUnits(code string) int {
	switch code {
	case "BIF", "CLP", "DJF", "GNF", "ISK", "JPY", "KMF", "KRW", "PYG", "RWF", "UGX", "VND", "VUV", "XAF", "XOF", "XPF":
		return 0
	case "BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND":
		return 3
	case "UYW":
		return 4
	default:
		return 2
	}
}

func CurrencyCatalog() []CurrencyDefinition {
	result := make([]CurrencyDefinition, len(currencyCatalog))
	copy(result, currencyCatalog)
	return result
}

func SelectableCurrencyCodes() []string {
	result := make([]string, 0, len(currentCurrencyCodes))
	for _, item := range currencyCatalog {
		if item.Selectable {
			result = append(result, item.Code)
		}
	}
	return result
}

func CurrencyMinorUnits(value string) (int, error) {
	value, err := NormalizeCurrency(value)
	if err != nil {
		return 0, err
	}
	return currencyExponents[value], nil
}

func NormalizeCurrency(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 3 {
		return "", errors.New("currency must be a three-letter ISO code")
	}
	for _, r := range value {
		if r > unicode.MaxASCII || !unicode.IsLetter(r) {
			return "", errors.New("currency must be a three-letter ISO code")
		}
	}
	if _, ok := currencyExponents[value]; !ok {
		return "", fmt.Errorf("unsupported currency %s", value)
	}
	return value, nil
}

func NormalizeSelectableCurrency(value string) (string, error) {
	value, err := NormalizeCurrency(value)
	if err != nil {
		return "", err
	}
	if !selectableCurrencies[value] {
		return "", fmt.Errorf("unsupported currency %s", value)
	}
	return value, nil
}
