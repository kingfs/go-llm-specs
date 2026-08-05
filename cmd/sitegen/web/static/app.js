const translations={
zh:{catalog:"模型目录",docs:"项目文档",heroTitle:"在一个目录里了解主流 LLM",heroCopy:"从项目维护的权威 YAML 数据自动生成，快速比较上下文窗口、最大输出、多模态和工具调用等关键能力。",copy:"复制",copied:"已复制",explore:"探索模型",catalogTitle:"模型目录",searchLabel:"搜索模型",searchPlaceholder:"搜索模型、厂商、ID 或别名",allProviders:"所有厂商",sortName:"按名称排序",sortContext:"上下文从大到小",sortOutput:"最大输出从大到小",clear:"清除筛选",emptyTitle:"没有匹配的模型",emptyCopy:"尝试清除部分筛选条件或使用其他关键词。",footer:"静态数据，随每次发布自动更新",models:"模型",providers:"厂商",multimodal:"多模态",toolUse:"工具调用",reasoning:"推理模型",results:"个模型",context:"上下文",output:"最大输出",unknown:"未公布",official:"官方页面",modelCard:"模型卡",documentation:"文档",paper:"论文",repository:"仓库",website:"厂商官网"},
en:{catalog:"Model catalog",docs:"Documentation",heroTitle:"Understand leading LLMs in one catalog",heroCopy:"Automatically generated from the project's authoritative YAML registry. Compare context windows, output limits, modalities and tool-use capabilities.",copy:"Copy",copied:"Copied",explore:"EXPLORE",catalogTitle:"Model catalog",searchLabel:"Search models",searchPlaceholder:"Search models, providers, IDs or aliases",allProviders:"All providers",sortName:"Sort by name",sortContext:"Largest context first",sortOutput:"Largest output first",clear:"Clear filters",emptyTitle:"No models found",emptyCopy:"Try removing a filter or using a different search term.",footer:"Static metadata, automatically updated with every release",models:"Models",providers:"Providers",multimodal:"Multimodal",toolUse:"Tool use",reasoning:"Reasoning",results:"models",context:"Context",output:"Max output",unknown:"Unpublished",official:"Official",modelCard:"Model card",documentation:"Docs",paper:"Paper",repository:"Repository",website:"Provider website"}
};
const tagLabels={chat:["对话","Chat"],embedding:["向量","Embedding"],rerank:["重排","Rerank"],tts:["语音合成","TTS"],asr:["语音识别","ASR"],"tool-use":["工具调用","Tool use"],"structured-output":["结构化输出","Structured output"],multimodal:["多模态","Multimodal"],vision:["图片输入","Vision"],"image-output":["图片生成","Image output"],"audio-input":["音频输入","Audio input"],"audio-output":["音频输出","Audio output"],"video-input":["视频输入","Video input"],"video-output":["视频输出","Video output"],"file-input":["文件输入","File input"],reasoning:["推理","Reasoning"],preview:["预览","Preview"],experimental:["实验性","Experimental"],free:["免费","Free"],fast:["快速","Fast"],mini:["Mini","Mini"],nano:["Nano","Nano"],turbo:["Turbo","Turbo"],pro:["Pro","Pro"],thinking:["思考","Thinking"]};
const filterTags=["chat","embedding","reasoning","tool-use","structured-output","multimodal","vision","image-output","audio-input","audio-output"];
const state={catalog:null,lang:localStorage.getItem("museum-language")||((navigator.language||"").toLowerCase().startsWith("zh")?"zh":"en"),query:"",provider:"",sort:"name",tags:new Set()};
const $=selector=>document.querySelector(selector);
const escapeHTML=value=>String(value||"").replace(/[&<>"']/g,char=>({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[char]));
const label=key=>translations[state.lang][key]||key;
const tagLabel=tag=>(tagLabels[tag]||[tag,tag])[state.lang==="zh"?0:1];
const compact=value=>{if(!value)return label("unknown");if(value>=1e6)return `${Number((value/1e6).toFixed(value%1e6?1:0))}M`;if(value>=1e3)return `${Number((value/1e3).toFixed(value%1e3?1:0))}K`;return value.toLocaleString()};

function applyLanguage(){
 document.documentElement.lang=state.lang==="zh"?"zh-CN":"en";$("#language").textContent=state.lang==="zh"?"EN":"中文";
 document.querySelectorAll("[data-i18n]").forEach(node=>node.textContent=label(node.dataset.i18n));
 document.querySelectorAll("[data-i18n-placeholder]").forEach(node=>node.placeholder=label(node.dataset.i18nPlaceholder));
 if(state.catalog){renderStats();renderProviderOptions();renderFilters();render()}
}
function renderStats(){const s=state.catalog.stats;$("#stats").innerHTML=[[s.models,"models"],[s.providers,"providers"],[s.multimodal,"multimodal"],[s.tool_use,"toolUse"],[s.reasoning,"reasoning"]].map(([value,key])=>`<div class="stat"><strong>${value.toLocaleString()}</strong><span>${label(key)}</span></div>`).join("")}
function renderProviderOptions(){const select=$("#provider"),value=state.provider;select.innerHTML=`<option value="">${label("allProviders")}</option>`+state.catalog.providers.map(p=>`<option value="${escapeHTML(p.id)}">${escapeHTML(p.name)}</option>`).join("");select.value=value}
function renderFilters(){$("#tag-filters").innerHTML=filterTags.map(tag=>`<button type="button" class="filter-chip${state.tags.has(tag)?" active":""}" data-filter="${tag}" aria-pressed="${state.tags.has(tag)}">${escapeHTML(tagLabel(tag))}</button>`).join("")}
function searchable(model){return [model.id,model.name,model.name_cn,model.provider,model.developer,model.description,model.description_cn,...(model.aliases||[])].join(" ").toLocaleLowerCase()}
function matches(model){const query=state.query.trim().toLocaleLowerCase();return(!query||searchable(model).includes(query))&&(!state.provider||model.provider_id===state.provider)&&[...state.tags].every(tag=>model.tags.includes(tag))}
function sorted(models){return models.sort((a,b)=>state.sort==="context"?b.context_length-a.context_length||a.name.localeCompare(b.name):state.sort==="output"?(b.max_output||0)-(a.max_output||0)||a.name.localeCompare(b.name):a.name.localeCompare(b.name,state.lang==="zh"?"zh":"en"))}
function providerLink(provider){return provider&&(provider.homepage||provider.model_catalog||provider.documentation)}
function render(){
 const models=sorted(state.catalog.models.filter(matches));const providers=new Map(state.catalog.providers.map(p=>[p.id,p]));const groups=new Map();
 for(const model of models){if(!groups.has(model.provider_id))groups.set(model.provider_id,[]);groups.get(model.provider_id).push(model)}
 $("#result-count").textContent=`${models.length.toLocaleString()} ${label("results")}`;$("#empty").hidden=models.length!==0;
 $("#groups").innerHTML=[...groups].map(([id,items])=>{const p=providers.get(id)||{id,name:items[0].provider};const link=providerLink(p);return `<section class="provider-group" id="provider-${escapeHTML(id)}"><div class="provider-heading"><h3>${escapeHTML(p.name)}</h3><span class="count">${items.length}</span>${link?`<a href="${escapeHTML(link)}" target="_blank" rel="noopener">${label("website")} ↗</a>`:""}</div><div class="grid">${items.map(card).join("")}</div></section>`}).join("")
 updateURL();
}
function card(model){
 const description=state.lang==="zh"?(model.description_cn||model.description):(model.description||model.description_cn);const links=model.links||{};const linkMap=[["official","official"],["model_card","modelCard"],["documentation","documentation"],["paper","paper"],["repository","repository"]];
 return `<article class="card"><div class="card-title"><h4>${escapeHTML(state.lang==="zh"&&model.name_cn?model.name_cn:model.name)}</h4><button class="copy-id" type="button" data-copy="${escapeHTML(model.id)}" title="Copy model ID">⧉</button></div><div class="model-id">${escapeHTML(model.id)}</div><p class="description">${escapeHTML(description)}</p><div class="facts"><div class="fact"><span>${label("context")}</span><strong>${compact(model.context_length)}</strong></div><div class="fact"><span>${label("output")}</span><strong>${compact(model.max_output)}</strong></div></div><div class="tags">${model.tags.slice(0,8).map(tag=>`<span class="tag" data-tag="${tag}">${escapeHTML(tagLabel(tag))}</span>`).join("")}</div><div class="card-links">${linkMap.filter(([key])=>links[key]).slice(0,3).map(([key,text])=>`<a href="${escapeHTML(links[key])}" target="_blank" rel="noopener">${label(text)} ↗</a>`).join("")}</div></article>`
}
function updateURL(){const params=new URLSearchParams();if(state.query)params.set("q",state.query);if(state.provider)params.set("provider",state.provider);if(state.tags.size)params.set("tags",[...state.tags].join(","));if(state.sort!=="name")params.set("sort",state.sort);history.replaceState(null,"",params.size?`?${params}#catalog`:location.pathname+location.hash)}
function restoreURL(){const params=new URLSearchParams(location.search);state.query=params.get("q")||"";state.provider=params.get("provider")||"";state.sort=params.get("sort")||"name";state.tags=new Set((params.get("tags")||"").split(",").filter(Boolean));$("#search").value=state.query;$("#sort").value=state.sort}

$("#language").addEventListener("click",()=>{state.lang=state.lang==="zh"?"en":"zh";localStorage.setItem("museum-language",state.lang);applyLanguage()});
$("#search").addEventListener("input",event=>{state.query=event.target.value;render()});
$("#provider").addEventListener("change",event=>{state.provider=event.target.value;render()});
$("#sort").addEventListener("change",event=>{state.sort=event.target.value;render()});
$("#tag-filters").addEventListener("click",event=>{const tag=event.target.dataset.filter;if(!tag)return;state.tags.has(tag)?state.tags.delete(tag):state.tags.add(tag);renderFilters();render()});
$("#clear").addEventListener("click",()=>{state.query="";state.provider="";state.sort="name";state.tags.clear();$("#search").value="";$("#sort").value="name";renderProviderOptions();renderFilters();render()});
$("#groups").addEventListener("click",async event=>{const id=event.target.dataset.copy;if(!id)return;await navigator.clipboard.writeText(id);event.target.textContent="✓";setTimeout(()=>event.target.textContent="⧉",1000)});
$("#copy-install").addEventListener("click",async event=>{await navigator.clipboard.writeText("go get github.com/kingfs/go-llm-specs");event.target.textContent=label("copied");setTimeout(()=>event.target.textContent=label("copy"),1000)});

fetch("catalog.json").then(response=>{if(!response.ok)throw new Error(`catalog: ${response.status}`);return response.json()}).then(catalog=>{state.catalog=catalog;restoreURL();applyLanguage()}).catch(error=>{$("#groups").innerHTML=`<div class="empty"><h3>Unable to load catalog</h3><p>${escapeHTML(error.message)}</p></div>`});
