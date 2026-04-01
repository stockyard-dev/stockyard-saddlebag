package server

import "net/http"

const uiHTML = `<!DOCTYPE html><html lang="en"><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Saddlebag — Stockyard</title>
<link href="https://fonts.googleapis.com/css2?family=Libre+Baskerville:wght@400;700&family=JetBrains+Mono:wght@400;600&display=swap" rel="stylesheet">
<style>:root{--bg:#1a1410;--bg2:#241e18;--bg3:#2e261e;--rust:#c45d2c;--rust-light:#e8753a;--rust-dark:#8b3d1a;--leather:#a0845c;--cream:#f0e6d3;--cream-dim:#bfb5a3;--cream-muted:#7a7060;--green:#5ba86e;--red:#c0392b;--font-serif:'Libre Baskerville',Georgia,serif;--font-mono:'JetBrains Mono',monospace}
*{margin:0;padding:0;box-sizing:border-box}body{background:var(--bg);color:var(--cream);font-family:var(--font-serif);min-height:100vh}a{color:var(--rust-light);text-decoration:none}
.hdr{background:var(--bg2);border-bottom:2px solid var(--rust-dark);padding:.9rem 1.8rem;display:flex;align-items:center;justify-content:space-between}.hdr-left{display:flex;align-items:center;gap:1rem}.hdr-brand{font-family:var(--font-mono);font-size:.75rem;color:var(--leather);letter-spacing:3px;text-transform:uppercase}.hdr-title{font-family:var(--font-mono);font-size:1.1rem;color:var(--cream)}.badge{font-family:var(--font-mono);font-size:.6rem;padding:.2rem .6rem;border:1px solid var(--green);color:var(--green);letter-spacing:1px;text-transform:uppercase}
.main{max-width:1000px;margin:0 auto;padding:2rem 1.5rem}.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:1rem;margin-bottom:2rem}.card{background:var(--bg2);border:1px solid var(--bg3);padding:1rem 1.2rem}.card-val{font-family:var(--font-mono);font-size:1.6rem;font-weight:700;display:block}.card-lbl{font-family:var(--font-mono);font-size:.58rem;letter-spacing:2px;text-transform:uppercase;color:var(--leather);margin-top:.2rem}
.section{margin-bottom:2rem}.section-title{font-family:var(--font-mono);font-size:.68rem;letter-spacing:3px;text-transform:uppercase;color:var(--rust-light);margin-bottom:.8rem;padding-bottom:.5rem;border-bottom:1px solid var(--bg3)}table{width:100%;border-collapse:collapse;font-family:var(--font-mono);font-size:.75rem}th{background:var(--bg3);padding:.4rem .8rem;text-align:left;color:#c4a87a;font-weight:400;font-size:.62rem;letter-spacing:1px;text-transform:uppercase}td{padding:.4rem .8rem;border-bottom:1px solid var(--bg3);color:var(--cream-dim)}tr:hover td{background:var(--bg2)}.empty{color:var(--cream-muted);text-align:center;padding:2rem;font-style:italic}
.btn{font-family:var(--font-mono);font-size:.7rem;padding:.3rem .8rem;border:1px solid var(--leather);background:transparent;color:var(--cream);cursor:pointer}.btn:hover{border-color:var(--rust-light);color:var(--rust-light)}.btn-rust{border-color:var(--rust);color:var(--rust-light)}.btn-rust:hover{background:var(--rust);color:var(--cream)}.btn-sm{font-size:.62rem;padding:.2rem .5rem}
.upload-box{background:var(--bg2);border:2px dashed var(--bg3);padding:2rem;text-align:center;margin-bottom:1.5rem;cursor:pointer}
.upload-box:hover{border-color:var(--leather)}
.upload-box input{display:none}
</style></head><body>
<div class="hdr"><div class="hdr-left">
<svg viewBox="0 0 64 64" width="22" height="22" fill="none"><rect x="8" y="8" width="8" height="48" rx="2.5" fill="#e8753a"/><rect x="28" y="8" width="8" height="48" rx="2.5" fill="#e8753a"/><rect x="48" y="8" width="8" height="48" rx="2.5" fill="#e8753a"/><rect x="8" y="27" width="48" height="7" rx="2.5" fill="#c4a87a"/></svg>
<span class="hdr-brand">Stockyard</span><span class="hdr-title">Saddlebag</span></div>
<div><span class="badge">Free</span></div></div>
<div class="main">
<div class="cards">
  <div class="card"><span class="card-val" id="s-files">—</span><span class="card-lbl">Files</span></div>
  <div class="card"><span class="card-val" id="s-size">—</span><span class="card-lbl">Storage</span></div>
  <div class="card"><span class="card-val" id="s-dl">—</span><span class="card-lbl">Downloads</span></div>
</div>
<div class="upload-box" onclick="document.getElementById('file-input').click()">
  <input type="file" id="file-input" onchange="uploadFile(this.files[0])">
  <div style="font-family:var(--font-mono);font-size:.8rem;color:var(--cream-muted)">Click to upload a file</div>
</div>
<div id="upload-result" style="margin-bottom:1rem"></div>
<div class="section"><div class="section-title">Files</div>
<table><thead><tr><th>Name</th><th>Size</th><th>Downloads</th><th>Link</th><th></th></tr></thead>
<tbody id="files-body"></tbody></table></div>
</div>
<script>
async function refresh(){
  try{const s=await(await fetch('/api/status')).json();document.getElementById('s-files').textContent=s.files||0;document.getElementById('s-size').textContent=fmtBytes(s.total_bytes||0);document.getElementById('s-dl').textContent=s.total_downloads||0;}catch(e){}
  try{const d=await(await fetch('/api/files')).json();const fs=d.files||[];const tb=document.getElementById('files-body');
  if(!fs.length){tb.innerHTML='<tr><td colspan="5" class="empty">No files yet.</td></tr>';return;}
  tb.innerHTML=fs.map(f=>'<tr><td style="color:var(--cream)">'+esc(f.filename)+'</td><td>'+fmtBytes(f.size_bytes)+'</td><td>'+f.downloads+(f.max_downloads?' / '+f.max_downloads:'')+'</td><td><a href="/d/'+f.id+'" style="font-size:.65rem">/d/'+f.id.slice(0,8)+'…</a></td><td><button class="btn btn-sm" onclick="del(\''+f.id+'\')">Del</button></td></tr>').join('');}catch(e){}
}
async function uploadFile(file){if(!file)return;const fd=new FormData();fd.append('file',file);const r=await fetch('/api/upload',{method:'POST',body:fd});const d=await r.json();if(r.ok){document.getElementById('upload-result').innerHTML='<span style="font-family:var(--font-mono);font-size:.75rem;color:var(--green)">Uploaded → <a href="'+d.download_url+'">'+d.download_url+'</a></span>';refresh();}else{document.getElementById('upload-result').innerHTML='<span style="color:var(--red)">'+esc(d.error)+'</span>';}}
async function del(id){if(!confirm('Delete?'))return;await fetch('/api/files/'+id,{method:'DELETE'});refresh();}
function fmtBytes(b){if(b>=1073741824)return(b/1073741824).toFixed(1)+'GB';if(b>=1048576)return(b/1048576).toFixed(1)+'MB';if(b>=1024)return(b/1024).toFixed(0)+'KB';return b+'B';}
function esc(s){const d=document.createElement('div');d.textContent=s||'';return d.innerHTML;}
refresh();setInterval(refresh,8000);
</script></body></html>`

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(uiHTML))
}
