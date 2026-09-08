(() => {
  const english = document.documentElement.lang.startsWith('en');
  const text = english ? {loading:'Loading…',failed:'Unable to load. Please retry.',retry:'Retry',cancel:'Cancel',saved:'Saved independently. Cancelling this editor will not undo that change.',login:'Session expired. Sign in in another tab, then retry.'} : {loading:'正在加载…',failed:'加载失败，请重试。',retry:'重试',cancel:'取消',saved:'子层修改已独立保存，取消当前编辑不会撤销该修改。',login:'登录已失效，请在新标签页登录后重试。'};
  let sequence = 0;
  const remotes = new Map();
  const observed = new WeakSet();
  const order = [];
  const panels = () => {
    for(let i=order.length-1;i>=0;i--)if(!order[i].open||!order[i].isConnected)order.splice(i,1);
    for(const dialog of document.querySelectorAll('dialog.drawer[open]'))if(!order.includes(dialog))order.push(dialog);
    return [...order];
  };
  const layout = () => {
    const opened = panels().filter(dialog=>!dialog.hasAttribute('data-stack-closing'));
    for(const dialog of opened) if(!remotes.has(dialog)&&['tag-editor','model-drawer','category-drawer','resource-editor','resource-upload','event-type-manage'].includes(dialog.id)) {
      remotes.set(dialog,{target:dialog.id,native:true,parent:opened[opened.indexOf(dialog)-1],key:entityKey(dialog.id,dialog.querySelector('form')?.action||location.href)});
      if(!observed.has(dialog)){observed.add(dialog);dialog.addEventListener('close',()=>{remotes.delete(dialog);layout();});}
    }
    opened.forEach((dialog, index) => {
      const panel = dialog.querySelector('.drawer-panel');
      if (!panel) return;
      const depth = opened.length - index - 1;
      const distance = innerWidth < 600 ? Math.min(24, Math.max(0,innerWidth-360)) : 64;
      panel.style.setProperty('--drawer-push', `${Math.min(depth*distance,128,Math.max(0,innerWidth-48))}px`);
      dialog.toggleAttribute('data-covered', depth > 0);
      dialog.dispatchEvent(new CustomEvent('drawer:visibility',{detail:{visible:depth===0}}));
    });
  };
  const namespace = (dialog) => {
    const prefix = `drawer-${++sequence}-`, ids = new Map();
    for (const node of [dialog,...dialog.querySelectorAll('[id]')]) if(node.id){ids.set(node.id,prefix+node.id);node.id=prefix+node.id;}
    for (const node of dialog.querySelectorAll('*')) for (const attr of ['for','form','aria-labelledby','aria-describedby','aria-controls']) {
      if(node.hasAttribute(attr)) node.setAttribute(attr,node.getAttribute(attr).split(' ').map(id=>ids.get(id)||id).join(' '));
    }
    for(const attr of ['aria-labelledby','aria-describedby']) if(dialog.hasAttribute(attr)) dialog.setAttribute(attr,dialog.getAttribute(attr).split(' ').map(id=>ids.get(id)||id).join(' '));
    for(const link of dialog.querySelectorAll('[href^="#"]'))if(ids.has(link.getAttribute('href').slice(1)))link.setAttribute('href','#'+ids.get(link.getAttribute('href').slice(1)));
  };
  function entityKey(target,href) {
    const url=new URL(href,location.href),id=url.searchParams.get('edit')||url.searchParams.get('edit_model_id')||url.searchParams.get('edit_type_id');
    if(target==='appearance-editor')return target+':'+url.pathname+':'+(url.searchParams.get('rule_id')||'configuration');
    if(id)return target+':'+id;
    const parts=url.pathname.split('/').filter(Boolean),last=parts.at(-1);
    if(['edit','binding'].includes(last))return target+':'+parts.at(-2);
    if(last && !['catalog','tags','types','new','3d','models','categories','event-types'].includes(last))return target+':'+last;
    url.searchParams.delete('dialog');url.searchParams.sort();return target+':'+url.pathname+url.search;
  }
  function feedback(dialog,message,retry) {
    let box=dialog.querySelector('[data-stack-feedback]');
    if(!box){box=document.createElement('div');box.dataset.stackFeedback='';box.setAttribute('role','status');dialog.querySelector('.drawer-body,.model-drawer-body,.drawer-panel').prepend(box);}
    box.replaceChildren(document.createTextNode(message));
    if(retry){const button=document.createElement('button');button.type='button';button.className='secondary auto';button.textContent=text.retry;button.onclick=retry;box.append(button);}
  }
  async function open(link) {
    const url=new URL(link.href,location.href);
    if(link.dataset.drawerField&&link.dataset.drawerTarget!=='resource-upload'){const value=link.closest('form,dialog')?.querySelector(`[name="${link.dataset.drawerField}"]`)?.value;if(value){if(link.dataset.drawerQuery)url.searchParams.set(link.dataset.drawerQuery,value);else url.pathname=url.pathname.replace(/[^/]+$/,encodeURIComponent(value));}}
    if(url.origin!==location.origin)return;
    const target=link.dataset.drawerTarget;
    const key=entityKey(target,url.href);
    const existing=[...remotes].find(([,v])=>v.key===key);
    if(existing){const list=panels();for(let i=list.length-1;i>=0&&list[i]!==existing[0];i--)if(!await window.assetloopDialog.close(list[i]))return;existing[0].querySelector('button')?.focus();return;}
    const parent=panels().at(-1);
    const dialog=document.createElement('dialog');dialog.className='drawer';dialog.dataset.managementDrawer='';dialog.dataset.stackRemote='true';
    const panel=document.createElement('div');panel.className='drawer-panel';
    dialog.dataset.drawerKind=target;
    const header=document.createElement('header');header.className='drawer-heading';const heading=document.createElement('h2');heading.textContent=text.loading;
    const close=document.createElement('button');close.type='button';close.className='secondary auto';close.dataset.dialogClose='';close.textContent=text.cancel;header.append(heading,close);panel.append(header);dialog.append(panel);
    const creating=/\/new$/.test(url.pathname)||(!url.searchParams.has('edit')&&!url.searchParams.has('edit_model_id')&&!url.searchParams.has('edit_type_id')&&['tag-editor','model-drawer','resource-upload','event-type-manage'].includes(target));
    const state={key,url:url.href,target,link,parent,creating,controller:null};remotes.set(dialog,state);document.body.append(dialog);
    window.assetloopDialog.initialize(dialog);dialog.showModal();layout();
    dialog.addEventListener('close',()=>{state.controller?.abort();dialog.dispatchEvent(new Event('drawer:dispose'));remotes.delete(dialog);dialog.remove();layout();const focus=link.isConnected?link:[...(parent?.querySelectorAll('a[href]')||[])].find(a=>a.href===link.href);focus?.focus({preventScroll:true});});
    const load=async(queryOnly=false)=>{
      state.controller?.abort();const controller=new AbortController();state.controller=controller;dialog.setAttribute('aria-busy','true');
      try{
        const response=await fetch(state.url,{cache:'no-store',signal:controller.signal,headers:{'X-Assetloop-Drawer':target},credentials:'same-origin'});
        if(response.status===401||new URL(response.url).pathname==='/login')throw Error('login');
        if(!response.ok)throw Error('http');
        const page=new DOMParser().parseFromString(await response.text(),'text/html');
        if(!dialog.open||state.controller!==controller)return;
        const incoming=page.querySelector('dialog.drawer');if(!incoming)throw Error('content');
        for(const script of incoming.querySelectorAll('script'))script.remove();
        if(queryOnly) {
          const results=[...incoming.querySelectorAll('[data-drawer-query-results]')];
          if(!results.length)throw Error('content');
          for(const next of results){const old=dialog.querySelector(`[data-drawer-query-results="${next.dataset.drawerQueryResults}"]`);if(old){
            for(const select of old.querySelectorAll('[data-drawer-query-select]')) {
              const replacement=next.querySelector(`[data-drawer-query-select="${select.dataset.drawerQuerySelect}"]`);
              if(replacement){const value=select.value,option=select.selectedOptions[0]?.cloneNode(true);if(option&&![...replacement.options].some(o=>o.value===value))replacement.append(option);select.replaceChildren(...replacement.childNodes);select.value=value;replacement.replaceWith(select);}
            }
            if(old.tagName==='SELECT') {const value=old.value,selected=old.selectedOptions[0]?.cloneNode(true);if(selected&&![...next.options].some(o=>o.value===value))next.append(selected);old.replaceChildren(...next.childNodes);old.value=value;}
            else {namespace(next);old.replaceWith(next);}
          }}
          for(const form of dialog.querySelectorAll('form[enctype="multipart/form-data"]')) {
            const fresh=[...incoming.querySelectorAll('form')].find(candidate=>candidate.action===form.action);
            if(!fresh)continue;
            for(const input of form.querySelectorAll('input[type="hidden"][name="tag_ids"]'))input.remove();
            for(const input of fresh.querySelectorAll('input[type="hidden"][name="tag_ids"]'))form.append(input.cloneNode(true));
            const submit=form.querySelector('[type="submit"]'),freshSubmit=fresh.querySelector('[type="submit"]');if(submit&&freshSubmit)submit.disabled=freshSubmit.disabled;
          }
          dialog.querySelector('[data-stack-feedback]')?.remove();return;
        }
        namespace(incoming);dialog.setAttribute('aria-labelledby',incoming.getAttribute('aria-labelledby')||'');
        dialog.replaceChildren(...incoming.childNodes);
        if(response.headers.get('X-Assetloop-Readonly')==='true')for(const input of dialog.querySelectorAll('input,select,textarea,button[type="submit"],[data-dialog-open]'))input.disabled=true;
        window.assetloopDialog.initialize(dialog);
        dialog.dispatchEvent(new CustomEvent('drawer:loaded',{bubbles:true}));
        if(dialog.querySelector('[data-asset-model]'))await import('./asset-tags.js');
        if(dialog.querySelector('[data-model-viewer]')) { const viewer=await import('./asset-model-viewer.js'); if(dialog.open)viewer.initializeViewers(dialog); }
        (dialog.querySelector('[data-dialog-initial-focus]')||dialog.querySelector('input:not([type="hidden"]),button'))?.focus({preventScroll:true});layout();
      }catch(error){if(error.name!=='AbortError'&&dialog.open){feedback(dialog,error.message==='login'?text.login:text.failed,()=>load(queryOnly));if(error.message==='login'){const a=document.createElement('a');a.href='/login';a.target='_blank';a.rel='noopener';a.textContent=english?'Sign in':'登录';dialog.querySelector('[data-stack-feedback]').append(a);}}}
      finally{if(state.controller===controller)dialog.removeAttribute('aria-busy');}
    };
    state.load=load;
    await load();
  }
  function sync(result,state) {
    const fieldName={category:'category_id',model:'model_id','tag-type':'type_id',resource:'resource_id','event-type':'event_type'}[result.kind];
    if(fieldName && state.parent && state.creating) for(const select of state.parent.querySelectorAll(`select[name="${fieldName}"]`)) {
      if(![...select.options].some(option=>option.value===result.id)){const option=new Option(result.name,result.id);if(result.cashflow)option.dataset.cashflow=result.cashflow;select.add(option);select.value=result.id;select.dispatchEvent(new Event('change',{bubbles:true}));}
    }
    for(const node of document.querySelectorAll('option,[data-entity-id]')) {
      if((node.value||node.dataset.entityId)!==result.id)continue;
      if(node.tagName==='OPTION')node.textContent=result.name;else if(node.dataset.entityName!==undefined)node.textContent=result.name;
      if(result.cashflow)node.dataset.cashflow=result.cashflow;
      if(!result.enabled)node.dataset.unavailable='true';else delete node.dataset.unavailable;
    }
    if(state.parent?.isConnected)feedback(state.parent,text.saved+(!result.enabled?(english?' This entry is disabled; retained selections will be checked when saving.':' 此项已停用，当前选择仍保留，保存时将校验是否可用。'):''));
    document.dispatchEvent(new CustomEvent('drawer:saved',{detail:{...result,parent:state.parent,created:state.creating}}));
    refreshRoot();
  }
  let refreshSerial=0;
  async function refreshRoot() {
    const root=document.getElementById('settings-content'),url=location.href,serial=++refreshSerial;
    if(!root)return;
    try {
      const response=await fetch(url,{credentials:'same-origin'});if(!response.ok)return;
      const next=new DOMParser().parseFromString(await response.text(),'text/html').getElementById('settings-content');
      if(!next||serial!==refreshSerial||location.href!==url||!root.isConnected)return;
      const selector=':scope > .table-card, :scope > .pagination, :scope > p.muted, :scope > .empty-state';
      const old=[...root.querySelectorAll(selector)],fresh=[...next.querySelectorAll(selector)];
      const x=scrollX,y=scrollY,anchor=old[0]||root.querySelector(':scope > dialog');
      for(const item of fresh)root.insertBefore(item,anchor);for(const item of old)item.remove();
      window.scrollTo(x,y);
    } catch { /* The saved child remains committed; retaining old results is safe. */ }
  }
  function submit(event) {
    const form=event.target,dialog=form.closest('dialog'),state=remotes.get(dialog);
    if(!state)return false;
    event.preventDefault();
    if(form.dataset.submitting==='true')return true;
    const data=new FormData(form);if(event.submitter?.name)data.append(event.submitter.name,event.submitter.value);
    if(form.method.toLowerCase()==='get') {
      const url=new URL(form.action);url.search=new URLSearchParams(data).toString();
      if(state.load){state.url=url.href;state.load(true);}
      return true;
    }
    form.dataset.submitting='true';dialog.setAttribute('aria-busy','true');
    const buttons=[...dialog.querySelectorAll('button')].map(b=>[b,b.disabled]);for(const [b]of buttons)b.disabled=true;
    (async()=>{
      try{
        const body=form.enctype==='multipart/form-data'?data:new URLSearchParams(data);
        const response=await fetch(form.action,{method:'POST',body,credentials:'same-origin',headers:{'X-Assetloop-Drawer':state.target}});
        if(response.ok&&response.headers.get('Content-Type')?.includes('application/json')){
          const result=await response.json();sync(result,state);form.dataset.dirty='false';delete form.dataset.submitting;await beforeClose(dialog);dialog.close();return;
        }
        if(response.status===401||new URL(response.url).pathname==='/login'){feedback(dialog,text.login);return;}
        const page=new DOMParser().parseFromString(await response.text(),'text/html');
        const errors=[...page.querySelectorAll('[data-error-summary],.error')].map(n=>n.textContent.trim()).filter(Boolean);
        feedback(dialog,errors.join(' · ')||text.failed);
      }catch{feedback(dialog,text.failed);}
      finally{delete form.dataset.submitting;dialog.removeAttribute('aria-busy');for(const [b,disabled]of buttons)b.disabled=disabled;}
    })();
    return true;
  }
  document.addEventListener('click',event=>{
    if(event.defaultPrevented||event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;
    const cancel=event.target.closest('[data-drawer-cancel]');if(cancel?.closest('dialog')){event.preventDefault();event.stopImmediatePropagation();window.assetloopDialog.close(cancel.closest('dialog'));return;}
    const local=event.target.closest('[data-dialog-open]');
    if(local && !local.hidden) {
      const target=local.dataset.dialogOpen;
      let href={'category-drawer':'/admin/catalog/categories/new','model-drawer':'/admin/catalog?dialog=model-drawer','resource-upload':'/admin/3d?dialog=resource-upload','event-type-manage':'/admin/event-types?dialog=event-type-manage'}[target];
      if(target==='model-drawer'&&local.dataset.editModelId)href+='&edit_model_id='+encodeURIComponent(local.dataset.editModelId);
      if(target==='category-drawer'&&local.dataset.action?.split('/').length===5)href=local.dataset.action;
      if(href){event.preventDefault();event.stopImmediatePropagation();open({href,dataset:{drawerTarget:target},get isConnected(){return local.isConnected;},focus:options=>local.focus(options)});return;}
    }
    const link=event.target.closest('a[data-drawer-target]');if(!link)return;
    event.preventDefault();event.stopImmediatePropagation();
    const state=remotes.get(link.closest('dialog'));
    if(link.closest('.pagination')&&state?.load){state.url=link.href;state.load(true);return;}
    open(link);
  },true);
  new MutationObserver(layout).observe(document.body,{subtree:true,attributes:true,attributeFilter:['open']});
  window.addEventListener('resize',layout);
  async function beforeClose(dialog) {
    remotes.get(dialog)?.controller?.abort();
    const returning=panels().filter(layer=>layer!==dialog&&!layer.hasAttribute('data-stack-closing'));
    for(const layer of returning)layer.setAttribute('data-stack-returning','');
    dialog.addEventListener('close',()=>{
      dialog.removeAttribute('data-stack-closing');
      for(const layer of returning)layer.removeAttribute('data-stack-returning');
    },{once:true});
    dialog.setAttribute('data-stack-closing','');layout();
    if(!matchMedia('(prefers-reduced-motion: reduce)').matches) {
      const animations=dialog.querySelector('.drawer-panel')?.getAnimations?.()||[];
      await Promise.allSettled(animations.map(animation=>animation.finished));
    }
  }
  window.assetloopDrawers={submit,open,beforeClose,opened(dialog){if(!order.includes(dialog))order.push(dialog);layout();}};
})();
