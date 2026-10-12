const $ = id => document.getElementById(id);
let files = [];

function host(u) { try { return new URL(u).hostname; } catch (e) { return ""; } }
function safe(s) { return s.replace(/[\\/:*?"<>|]+/g, "_").replace(/\s+/g, " ").trim().slice(0, 120) || "video"; }
function withExt(n) { return /\.(mp4|webm|mov|m4v|mkv|ogv|ogg)$/i.test(n) ? n : n + ".mp4"; }

async function scan() {
  $("sub").textContent = "Procurando vídeos…";
  $("list").innerHTML = ""; $("status").textContent = "";
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  let results = [];
  try {
    await chrome.scripting.executeScript({ target: { tabId: tab.id, allFrames: true }, files: ["scan.js"] });
    results = await chrome.scripting.executeScript({ target: { tabId: tab.id, allFrames: true }, func: () => window.__zaptakuScan() });
  } catch (e) {
    $("sub").textContent = "Esta página não pode ser lida (página interna do Chrome ou loja de extensões).";
    return;
  }
  const map = new Map(); let prot = 0;
  for (const r of results) {
    if (!r || !r.result) continue;
    prot += r.result.protectedCount;
    for (const f of r.result.files) if (!map.has(f.url)) map.set(f.url, f);
  }
  files = [...map.values()];
  render(prot);
}

function render(prot) {
  const n = files.length;
  $("sub").textContent = n ? `${n} vídeo(s) encontrado(s) nesta página` : "Nenhum vídeo baixável encontrado.";
  $("bar").hidden = !n;
  $("prot").hidden = !prot;
  $("prot").textContent = `${prot} vídeo(s) em stream protegido/fatiado (ex.: YouTube, Crunchyroll, Netflix) — não são baixados.`;
  const ul = $("list");
  files.forEach((f, i) => {
    const li = document.createElement("li");
    const cb = document.createElement("input"); cb.type = "checkbox"; cb.checked = true; cb.dataset.i = i;
    cb.addEventListener("change", update);
    const info = document.createElement("div"); info.className = "info";
    const nm = document.createElement("div"); nm.className = "name"; nm.textContent = f.name;
    const mt = document.createElement("div"); mt.className = "meta";
    mt.textContent = [host(f.url), f.w && f.h ? `${f.w}x${f.h}` : "", f.kind === "link" ? "link" : f.kind === "meta" ? "meta" : "player"].filter(Boolean).join(" · ");
    info.append(nm, mt);
    const st = document.createElement("span"); st.className = "st"; st.id = "st" + i;
    li.append(cb, info, st); ul.append(li);
  });
  update();
}

function selected() { return [...document.querySelectorAll("#list input:checked")].map(c => files[+c.dataset.i]); }
function update() {
  const k = selected().length;
  $("go").disabled = !k;
  $("go").textContent = k === files.length ? `Baixar todos (${k})` : `Baixar selecionados (${k})`;
  $("all").checked = k === files.length;
}

$("all").addEventListener("change", e => { document.querySelectorAll("#list input").forEach(c => (c.checked = e.target.checked)); update(); });
$("rescan").addEventListener("click", scan);

$("go").addEventListener("click", async () => {
  const sel = [...document.querySelectorAll("#list input:checked")].map(c => +c.dataset.i);
  $("go").disabled = true;
  let ok = 0, fail = 0;
  for (const i of sel) {
    const f = files[i];
    const folder = "Zaptaku Videos/" + safe(host(f.url));
    try {
      await chrome.downloads.download({ url: f.url, filename: `${folder}/${withExt(safe(f.name))}`, conflictAction: "uniquify", saveAs: false });
      $("st" + i).textContent = "✅"; ok++;
    } catch (e) {
      $("st" + i).textContent = "⚠️"; $("st" + i).title = e.message; fail++;
    }
    $("status").textContent = `Enviando para downloads… ${ok + fail}/${sel.length}`;
  }
  $("status").textContent = `${ok} download(s) iniciado(s)${fail ? `, ${fail} com erro` : ""}. Pasta: Downloads/Zaptaku Videos`;
  update();
});

scan();
