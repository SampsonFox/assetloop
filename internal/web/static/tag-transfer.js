import {createTagTransferState, selectTransferRows} from './tag-transfer-state.mjs';

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
  const root=node('section','tag-transfer'); root.setAttribute('aria-label',title); root.dataset.transferUi='';
  const heading=node('div','field-heading transfer-heading');heading.append(node('h4','',title));
  if(reset){
    const restore=button('',()=>reset());restore.className='icon-button';restore.title=text.reset;restore.setAttribute('aria-label',text.reset);
    const svg=document.createElementNS('http://www.w3.org/2000/svg','svg'),path=document.createElementNS('http://www.w3.org/2000/svg','path');
    svg.setAttribute('viewBox','0 0 24 24');svg.setAttribute('aria-hidden','true');path.setAttribute('d','M3 10a9 9 0 1 1 2.6 8.4M3 4v6h6');svg.append(path);restore.append(svg);heading.append(restore);
  }
  root.append(heading);
  const tabs=node('div','transfer-mobile-tabs'), grid=node('div','transfer-grid'), arrows=node('div','transfer-arrows');
  const selections=[new Set(),new Set()], anchors=[null,null], lists=[], searches=[], counters=[], movers=[];
  let mobileSide=0;
  const refreshSide=()=>{
    root.dataset.side=String(mobileSide);
    [...tabs.children].forEach((b,i)=>b.setAttribute('aria-pressed',String(i===mobileSide)));
  };
  const visible=i=>items().filter(item=>Number(item.right)===i && (item.name+' '+(item.group||'')).toLocaleLowerCase().includes(searches[i].value.trim().toLocaleLowerCase()));
  const update=i=>{
    for(const row of lists[i].querySelectorAll('[data-choice]')) row.setAttribute('aria-pressed',String(selections[i].has(row.dataset.choice)));
    counters[i].textContent=String(visible(i).length);
    movers[i].disabled=!selections[i].size;
  };
  const choose=(i,id,event)=>{
    anchors[i]=selectTransferRows(selections[i],visible(i).filter(x=>!x.disabled).map(x=>x.id),id,anchors[i],{
      range:event.shiftKey, toggle:event.ctrlKey||event.metaKey||event.pointerType==='touch'||event.pointerType==='pen',
    });
    update(i);
  };
  const transferSelection=(i,ids=[...selections[i]])=>{
    if(!ids.length)return;
    selections[0].clear();selections[1].clear();anchors.fill(null);
    move(ids,i===0); render(); searches[i].focus({preventScroll:true});
  };
  titles.forEach((label,i)=>{
    tabs.append(button(label,()=>{mobileSide=i;refreshSide();}));
    const panel=node('div','transfer-panel'), header=node('div','transfer-header'), count=node('span','muted');
    header.append(node('span','',label),count);
    const search=node('input','transfer-search');search.type='search';search.placeholder=text.search;search.setAttribute('aria-label',label+' · '+text.search);
    const list=node('div','transfer-list');list.setAttribute('role','group');list.setAttribute('aria-label',label);
    const mover=button('',()=>transferSelection(i));mover.className='icon-button';mover.title=i===0?text.add:text.remove;mover.setAttribute('aria-label',mover.title);
    const svg=document.createElementNS('http://www.w3.org/2000/svg','svg'),path=document.createElementNS('http://www.w3.org/2000/svg','path');
    svg.setAttribute('viewBox','0 0 24 24');svg.setAttribute('aria-hidden','true');path.setAttribute('d',i===0?'M5 12h14m-6-6 6 6-6 6':'M19 12H5m6-6-6 6 6 6');svg.append(path);mover.append(svg);
    arrows.append(mover); panel.append(header,search,list);panel.dataset.column=String(i);grid.append(panel);
    lists.push(list);searches.push(search);counters.push(count);movers.push(mover);
    search.addEventListener('input',event=>{event.stopPropagation();selections[i].clear();anchors[i]=null;render();});
    search.addEventListener('change',event=>event.stopPropagation());
    search.addEventListener('keydown',event=>{if(event.key==='Enter') event.preventDefault();});
  });
  grid.insertBefore(arrows,grid.children[1]);
  const hint=node('p','muted transfer-hint');
  hint.append(node('span','transfer-desktop-hint',en?'Double-click to move · Ctrl/Cmd to select multiple':'双击移动 · Ctrl 多选'),node('span','transfer-touch-hint',en?'Tap to select · Use arrows to move':'点击多选 · 箭头移动'));
  hint.title=en?'Shift: range · Arrow keys: navigate · Space: select · Enter: move · Touch: tap to toggle':'Shift 连选 · 方向键导航 · 空格选择 · Enter 移动 · 触屏点击多选';
  root.append(tabs,grid,hint);refreshSide();
  function render() {
    const current=items();
    for(let i=0;i<2;i++){
      const valid=new Set(current.filter(v=>Number(v.right)===i&&!v.disabled).map(v=>v.id));
      for(const id of selections[i])if(!valid.has(id))selections[i].delete(id);
      const scroll=lists[i].scrollTop;lists[i].replaceChildren();
      const rows=visible(i),groups=new Map();
      for(const item of rows){
        let parent=lists[i];
        if(item.group){
          if(!groups.has(item.group)){
            const group=node('div','transfer-group');group.append(node('p','transfer-group-title',item.group));
            lists[i].append(group);groups.set(item.group,group);
          }
          parent=groups.get(item.group);
        }
        const row=node('div','transfer-row'),choice=button('',event=>choose(i,item.id,event));
        choice.className='transfer-choice';choice.dataset.choice=item.id;choice.disabled=!!item.disabled;
        choice.append(node('span','',item.name));if(item.note)choice.append(node('small','muted',item.note));
        choice.addEventListener('dblclick',event=>{event.preventDefault();transferSelection(i,[item.id]);});
        choice.addEventListener('keydown',event=>{
          if(event.key==='Enter'){event.preventDefault();transferSelection(i,selections[i].has(item.id)?[...selections[i]]:[item.id]);return;}
          if(event.key===' '){event.preventDefault();choose(i,item.id,{ctrlKey:true});return;}
          const buttons=[...lists[i].querySelectorAll('[data-choice]:not(:disabled)')],index=buttons.indexOf(choice);
          const next=event.key==='ArrowDown'?Math.min(index+1,buttons.length-1):event.key==='ArrowUp'?Math.max(0,index-1):event.key==='Home'?0:event.key==='End'?buttons.length-1:-1;
          if(next<0)return;
          event.preventDefault();buttons[next].focus();
          if(event.shiftKey||!event.ctrlKey&&!event.metaKey)choose(i,buttons[next].dataset.choice,event);
        });
        row.append(choice);
        parent.append(row);
      }
      if(!rows.length)lists[i].append(node('p','transfer-empty muted',text.empty));
      lists[i].scrollTop=scroll;update(i);
    }
  }
  render();return {root,render};
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
      ()=>state.active().map(d=>({id:d.id,name:d.name,right:state.affects(d),
        note:d.appearance?text.inherited:''})),
      (ids,right)=>{state.moveAppearance(ids,right);sync();},
      ()=>{if(state.overrides.size){state.resetAllAppearance();sync();}});
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
