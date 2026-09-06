// The two transfers share persisted tag IDs, never display-name keys.
export function selectTransferRows(selected, visible, id, anchor, {range=false,toggle=false}={}) {
  if (!visible.includes(id)) return anchor;
  if (range && visible.includes(anchor)) {
    if (!toggle) selected.clear();
    const a=visible.indexOf(anchor), b=visible.indexOf(id);
    for (const key of visible.slice(Math.min(a,b),Math.max(a,b)+1)) selected.add(key);
    return anchor;
  }
  if (!toggle) selected.clear();
  if (toggle && selected.has(id)) selected.delete(id); else selected.add(id);
  return id;
}
export function createTagTransferState(dimensions) {
  const selected = new Set(dimensions.flatMap(d => d.choices.filter(c => c.selected).map(c => c.id)));
  const overrides = new Map(dimensions.filter(d => d.override).map(d => [d.id, d.override]));
  const active = () => dimensions.filter(d => d.choices.some(c => selected.has(c.id)));
  const prune = () => {
    const ids = new Set(active().map(d => d.id));
    for (const id of overrides.keys()) if (!ids.has(id)) overrides.delete(id);
  };
  return {
    selected, overrides, active,
    affects: d => overrides.has(d.id) ? overrides.get(d.id) === 'yes' : d.appearance,
    moveTags(ids, right) {
      const moving = new Set(ids);
      for (const d of dimensions) for (const c of d.choices) {
        if (!moving.has(c.id)) continue;
        if (!right) selected.delete(c.id);
        else if (c.enabled || c.selected) selected.add(c.id);
      }
      prune();
    },
    moveAppearance(ids, right) {
      const moving = new Set(ids);
      for (const d of active()) if (moving.has(d.id)) overrides.set(d.id, right ? 'yes' : 'no');
    },
    resetAppearance(id) { overrides.delete(id); },
    resetAllAppearance() { overrides.clear(); },
  };
}
