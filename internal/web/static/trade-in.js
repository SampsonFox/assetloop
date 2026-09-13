// Progressive enhancement for the trade-in review form: each asset money group
// reveals its FX evidence only for a foreign currency. The native form remains
// the submission source and works without this script.
import {tradeInFXVisible} from './trade-in-state.mjs';

const syncGroup = (group) => {
  const currency = group.querySelector('[data-trade-in-currency]');
  if (!currency) return;
  const foreign = tradeInFXVisible(currency.value, currency.dataset.tradeInBase);
  for (const field of group.querySelectorAll('[data-trade-in-fx]')) field.hidden = !foreign;
};

const syncAll = () => {
  for (const group of document.querySelectorAll('[data-trade-in-money]')) syncGroup(group);
};

const initialized = new WeakSet();
const initialize = (root = document) => {
  const groups = root.querySelectorAll('[data-trade-in-money]');
  groups.forEach((group) => {
    syncGroup(group);
    if (initialized.has(group)) return;
    initialized.add(group);
    const currency = group.querySelector('[data-trade-in-currency]');
    if (currency) {
      currency.addEventListener('input', () => syncGroup(group));
      currency.addEventListener('change', () => syncGroup(group));
    }
  });
};

document.addEventListener('settings:loaded', () => initialize());
initialize();
