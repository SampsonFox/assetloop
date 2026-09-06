import {createTagTransferState} from './tag-transfer-state.mjs';

const initialized = new WeakSet();
const en = document.documentElement.lang === 'en';
const text = en ? {
  tags:'Allowed tags', appearance:'Appearance dimensions', available:'Available tags', allowed:'Allowed here',
  no:'Does not affect appearance', yes:'Affects appearance', search:'Search type or tag…', all:'Select visible',
  add:'Move selected right', remove:'Move selected left', empty:'No matching options', reset:'Restore default',
  inherited:'Default', custom:'Custom', disabled:'Disabled', count:'Selected', done:'Selection updated',
  help:'Select tags above first. Moving a dimension overrides its default for this model only.',
} : {
  tags:'允许标签', appearance:'外观维度', available:'可选标签', allowed:'此型号已允许',
  no:'不影响外观', yes:'影响外观', search:'搜索类型或标签…', all:'选择当前结果',
  add:'移入右侧', remove:'移回左侧', empty:'没有匹配项', reset:'恢复默认',
  inherited:'默认', custom:'自定义', disabled:'已停用', count:'已勾选', done:'选择已更新',
  help:'先选择允许标签。移动维度只覆盖此型号，恢复默认后继续跟随标签类型。',
};
function node(tag, cls, value) {
  const element = document.createElement(tag);
  if (cls) element.className = cls;
  if (value) element.textContent = value;
  return element;
}
function button(label, action) {
  const b = node('button', 'secondary auto', label);
  b.type = 'button'; b.addEventListener('click', action); return b;
}
function transfer(title, titles, items, move, reset) {
  const root = node('section','tag-transfer');
  root.setAttribute('aria-label', title);
  root.dataset.transferUi = '';
  root.append(node('h4','',title));
  const tabs = node('div','transfer-mobile-tabs');
  const grid = node('div','transfer-grid');
  const panels = [], selections = [new Set(),new Set()], lists = [], searches = [], allBoxes = [], counters = [], movers = [];
  let mobileSide = 0;
  const refreshSide = () => {
    root.dataset.side = String(mobileSide);
    [...tabs.children].forEach((b,i) => b.setAttribute('aria-pressed', String(i===mobileSide)));
  };
  titles.forEach((label,i) => {
    tabs.append(button(label, () => {mobileSide=i;refreshSide();}));
    const panel = node('div','transfer-panel');
    const header=node('div','transfer-header'), allLabel=node('label','checkbox');
    const all=node('input'); all.type='checkbox'; all.setAttribute('aria-label', label+' · '+text.all);
    const count=node('span','muted');
    allLabel.append(all,node('span','',label)); header.append(allLabel,count);
    const search=node('input','transfer-search'); search.type='search'; search.placeholder=text.search;
    search.setAttribute('aria-label',label+' · '+text.search);
    const list=node('div','transfer-list'); list.setAttribute('role','group'); list.setAttribute('aria-label',label);
    const footer=node('div','transfer-footer'), selectedCount=node('span','muted');
    const mover=button(i===0?text.add:text.remove,()=>{
      move([...selections[i]],i===0);
      selections[0].clear(); selections[1].clear();
      render();
      searches[i].focus({preventScroll:true});
    });
    footer.append(selectedCount,mover);panel.append(header,search,list,footer);
    panel.dataset.column=String(i);grid.append(panel);
    panels.push(panel);lists.push(list);searches.push(search);allBoxes.push(all);counters.push([count,selectedCount]);movers.push(mover);
    search.addEventListener('input',event=>{event.stopPropagation();render();});
    search.addEventListener('change',event=>event.stopPropagation());
    search.addEventListener('keydown',event=>{if(event.key==='Enter') event.preventDefault();});
    all.addEventListener('change',event=>{
      event.stopPropagation();
      for(const item of visible(i)) if(!item.disabled) {
        if(all.checked) selections[i].add(item.id);else selections[i].delete(item.id);
      }
      render();
    });
  });
  root.append(tabs,grid);refreshSide();
  const visible = i => {
    const q=searches[i].value.trim().toLocaleLowerCase();
    return items().filter(item=>Number(item.right)===i && (item.name+' '+(item.group||'')).toLocaleLowerCase().includes(q));
  };
  const updateCounts = i => {
    const eligible=visible(i).filter(item=>!item.disabled);
    const checked=eligible.filter(item=>selections[i].has(item.id)).length;
    allBoxes[i].checked=eligible.length>0 && checked===eligible.length;
    allBoxes[i].indeterminate=checked>0 && checked<eligible.length;
    allBoxes[i].disabled=!eligible.length;
    counters[i][0].textContent=String(visible(i).length);
    counters[i][1].textContent=text.count+' '+selections[i].size;
    movers[i].disabled=!selections[i].size;
  };
  function render() {
    const current=items();
    for(let i=0;i<2;i++) {
      const valid=new Set(current.filter(v=>Number(v.right)===i&&!v.disabled).map(v=>v.id));
      for(const id of selections[i]) if(!valid.has(id)) selections[i].delete(id);
      const scroll=lists[i].scrollTop;lists[i].replaceChildren();
      const rows=visible(i);
      const groups=new Map();
      for(const item of rows) {
        let parent=lists[i];
        if(item.group) {
          if(!groups.has(item.group)) {
            const group=node('div','transfer-group');
            group.append(node('p','transfer-group-title',item.group));
            lists[i].append(group);groups.set(item.group,group);
          }
          parent=groups.get(item.group);
        }
        const row=node('div','transfer-row'), label=node('label','checkbox'), check=node('input');
        check.type='checkbox';check.checked=selections[i].has(item.id);check.disabled=!!item.disabled;
        label.append(check,node('span','',item.name));
        row.append(label);
        if(item.note) row.append(node('small','muted',item.note));
        if(item.custom && reset) row.append(button(text.reset,()=>{reset(item.id);searches[i].focus({preventScroll:true});}));
        check.addEventListener('change',event=>{
          event.stopPropagation();
          if(check.checked) selections[i].add(item.id);else selections[i].delete(item.id);
          updateCounts(i);
        });
        parent.append(row);
      }
      if(!rows.length) lists[i].append(node('p','transfer-empty muted',text.empty));
      lists[i].scrollTop=scroll;updateCounts(i);
    }
  }
  render();
  return {root,render};
}
function initialize() {
  for(const form of document.querySelectorAll('form.model-tag-editor')) {
    if(initialized.has(form)) continue;
    const source=form.querySelector('[data-transfer-source]');
    if(!source) continue;
    const dimensions=[...source.querySelectorAll('[data-tag-dimension]')].map(group=>({
      id:group.dataset.tagDimension,name:group.querySelector('legend').textContent,
      appearance:group.dataset.appearance==='true',
      override:group.querySelector('select').value,
      select:group.querySelector('select'),
      choices:[...group.querySelectorAll('[name="tag_ids"]')].map(input=>({
        id:input.value,name:input.dataset.label,enabled:input.dataset.enabled==='true',selected:input.checked,input,
      })),
    }));
    if(!dimensions.length) continue;
    let state=createTagTransferState(dimensions);
    const status=node('p','muted transfer-status');status.setAttribute('role','status');
    const sync = (dirty=true) => {
      for(const d of dimensions) {
        d.select.value=state.overrides.get(d.id)||'';
        for(const c of d.choices)c.input.checked=state.selected.has(c.id);
      }
      tags.render();appearance.render();
      form.dataset.dirty=String(dirty);
      status.textContent=text.done+' · '+text.allowed+' '+state.selected.size+' · '+text.appearance+' '+state.active().length;
    };
    const tags=transfer(text.tags,[text.available,text.allowed],
      ()=>dimensions.flatMap(d=>d.choices.map(c=>({id:c.id,name:c.name,group:d.name,right:state.selected.has(c.id),
        disabled:!state.selected.has(c.id)&&!c.enabled&&!c.selected,note:!c.enabled?text.disabled:''}))),
      (ids,right)=>{state.moveTags(ids,right);sync();});
    const appearance=transfer(text.appearance,[text.no,text.yes],
      ()=>state.active().map(d=>({id:d.id,name:d.name,right:state.affects(d),custom:state.overrides.has(d.id),
        note:state.overrides.has(d.id)?text.custom:text.inherited})),
      (ids,right)=>{state.moveAppearance(ids,right);sync();},
      id=>{state.resetAppearance(id);sync();});
    const help=node('p','muted transfer-help',text.help);
    tags.root.querySelector('h4').hidden=true;
    form.addEventListener('reset',()=>queueMicrotask(()=>{
      state=createTagTransferState(dimensions);sync(false);status.textContent='';
    }));
    source.before(tags.root,help,appearance.root,status);
    source.hidden=true;
    initialized.add(form);
  }
}
document.addEventListener('settings:loaded',initialize);
initialize();
