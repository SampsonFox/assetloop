// Pure trade-in presentation state shared by the browser enhancement and its
// Node interaction test. No DOM access here, so the behavior is testable and the
// transport never re-implements business policy.

// The two neutral pairing system types map to the documented command direction:
// a trade-in source event belongs to the NEW asset, a destination event to the
// OLD one.
export const TRADE_IN_DIRECTION_BY_SYSTEM_CODE = {
  trade_in_source: 'source',
  trade_in_destination: 'destination',
};

export function tradeInDirectionForSystemCode(systemCode) {
  return TRADE_IN_DIRECTION_BY_SYSTEM_CODE[String(systemCode ?? '').trim()] || '';
}

// tradeInNavigationTarget returns the dedicated trade-in form for a selected
// pairing type, or an empty string when the option is an ordinary event type.
export function tradeInNavigationTarget(base, systemCode) {
  const direction = tradeInDirectionForSystemCode(systemCode);
  if (!direction || !base) return '';
  return `${String(base)}?direction=${direction}`;
}

// A money group reveals its FX evidence only when the original currency is not
// the tenant base currency, matching the shared validation.
export function tradeInFXVisible(currency, baseCurrency) {
  const normalized = String(currency ?? '').trim().toUpperCase();
  const base = String(baseCurrency ?? '').trim().toUpperCase();
  if (!normalized || !base) return false;
  return normalized !== base;
}
