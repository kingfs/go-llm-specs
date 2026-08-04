package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/kingfs/go-llm-specs/internal/provider"
	"github.com/kingfs/go-llm-specs/internal/registry"
)

type siteCatalog struct {
	Providers []provider.Provider `json:"providers"`
	Models    []registry.Model    `json:"models"`
}

func main() {
	providersDir := flag.String("providers-dir", "providers", "publisher catalog directory")
	modelsDir := flag.String("models-dir", "models", "model registry directory")
	outputDir := flag.String("output-dir", "site", "generated static site directory")
	flag.Parse()
	providers, err := provider.Scan(*providersDir)
	if err != nil {
		log.Fatal(err)
	}
	models, err := registry.Scan(*modelsDir)
	if err != nil {
		log.Fatal(err)
	}
	for i := range models {
		models[i].FilePath = ""
	}
	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		log.Fatal(err)
	}
	data, err := json.Marshal(siteCatalog{Providers: providers, Models: models})
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*outputDir, "catalog.json"), append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*outputDir, "index.html"), []byte(indexHTML), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("generated site with %d publishers and %d models", len(providers), len(models))
}

const indexHTML = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>LLM Model Museum</title><style>
body{font:15px system-ui,sans-serif;max-width:1100px;margin:auto;padding:32px;color:#18212f;background:#f7f8fa}h1{margin-bottom:4px}input{width:100%;box-sizing:border-box;padding:12px;border:1px solid #ccd3dc;border-radius:8px}.meta{color:#657080}.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(300px,1fr));gap:12px;margin-top:20px}.card{background:white;border:1px solid #e1e5ea;border-radius:10px;padding:16px}.card h2{font-size:17px;margin:0 0 6px}a{color:#1769aa}</style></head>
<body><h1>LLM Model Museum</h1><p class="meta" id="summary">加载模型目录…</p><input id="q" placeholder="搜索模型、厂商或 ID"><div class="grid" id="models"></div>
<script>
let catalog;const esc=s=>String(s||'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function render(){const q=document.querySelector('#q').value.toLowerCase();const rows=catalog.models.filter(m=>[m.id,m.name,m.provider,m.developer].join(' ').toLowerCase().includes(q)).slice(0,200);document.querySelector('#models').innerHTML=rows.map(m=>'<article class="card"><h2>'+esc(m.name)+'</h2><div class="meta">'+esc(m.provider)+' · '+esc(m.id)+(m.lifecycle?' · '+esc(m.lifecycle):'')+'</div><p>上下文 '+Number(m.context_length||0).toLocaleString()+' · 最大输出 '+Number(m.max_output||0).toLocaleString()+'</p>'+(m.links&&m.links.official?'<a href="'+esc(m.links.official)+'">官方页面</a>':'')+(m.links&&m.links.model_card?' <a href="'+esc(m.links.model_card)+'">模型卡</a>':'')+'</article>').join('')}
fetch('catalog.json').then(r=>r.json()).then(x=>{catalog=x;document.querySelector('#summary').textContent=x.models.length+' 个历史模型 · '+x.providers.length+' 个已建档厂商';render()});document.querySelector('#q').addEventListener('input',render);
</script></body></html>`
