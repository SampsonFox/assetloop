package domain

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

type Money struct {
	Minor    int64
	Currency string
}

const FXRateScale int64 = 100_000_000

func ParseMajorAmount(value, currency string) (int64, error) {
	currency, err := NormalizeCurrency(currency)
	if err != nil {
		return 0, err
	}
	return parseDecimal(value, currencyExponents[currency])
}

func ParseFXRate(value string) (int64, error) {
	rate, err := parseDecimal(value, 8)
	if err != nil {
		return 0, fmt.Errorf("invalid FX rate: %w", err)
	}
	if rate <= 0 {
		return 0, errors.New("FX rate must be positive")
	}
	return rate, nil
}

func ConvertMinor(originalMinor int64, originalCurrency, baseCurrency string, rateScaled int64) (int64, error) {
	originalCurrency, err := NormalizeCurrency(originalCurrency)
	if err != nil {
		return 0, err
	}
	baseCurrency, err = NormalizeCurrency(baseCurrency)
	if err != nil {
		return 0, err
	}
	if originalMinor <= 0 || rateScaled <= 0 {
		return 0, errors.New("amount and FX rate must be positive")
	}
	numerator := new(big.Int).Mul(big.NewInt(originalMinor), big.NewInt(rateScaled))
	numerator.Mul(numerator, pow10(currencyExponents[baseCurrency]))
	denominator := new(big.Int).Mul(big.NewInt(FXRateScale), pow10(currencyExponents[originalCurrency]))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() || quotient.Int64() > math.MaxInt64 {
		return 0, errors.New("converted amount exceeds supported range")
	}
	return quotient.Int64(), nil
}

func FormatMinor(minor int64, currency string) string {
	currency, err := NormalizeCurrency(currency)
	if err != nil {
		return strconv.FormatInt(minor, 10) + " " + strings.TrimSpace(currency)
	}
	exponent := currencyExponents[currency]
	negative := minor < 0
	value := big.NewInt(minor)
	value.Abs(value)
	digits := value.String()
	if exponent > 0 {
		if len(digits) <= exponent {
			digits = strings.Repeat("0", exponent-len(digits)+1) + digits
		}
		digits = digits[:len(digits)-exponent] + "." + digits[len(digits)-exponent:]
	}
	if negative {
		digits = "-" + digits
	}
	return digits + " " + currency
}

func parseDecimal(value string, scaleDigits int) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("amount is required")
	}
	if strings.HasPrefix(value, "+") {
		value = value[1:]
	}
	if strings.HasPrefix(value, "-") {
		return 0, errors.New("amount must not be negative")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, errors.New("invalid decimal amount")
	}
	for _, part := range parts {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, errors.New("invalid decimal amount")
			}
		}
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > scaleDigits {
		return 0, fmt.Errorf("amount supports at most %d decimal places", scaleDigits)
	}
	fraction += strings.Repeat("0", scaleDigits-len(fraction))
	digits := strings.TrimLeft(parts[0]+fraction, "0")
	if digits == "" {
		return 0, nil
	}
	parsed := new(big.Int)
	if _, ok := parsed.SetString(digits, 10); !ok || !parsed.IsInt64() {
		return 0, errors.New("amount exceeds supported range")
	}
	return parsed.Int64(), nil
}

func pow10(exponent int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
}
