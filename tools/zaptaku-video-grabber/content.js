// Atualiza o contador no ícone da extensão conforme a página carrega vídeos.
(function () {
  if (window.__zaptakuContent) return; window.__zaptakuContent = true;
  let last = -1, timer = null;
  function report() {
    timer = null;
    try {
      const r = window.__zaptakuScan();
      const n = r.files.length;
      if (n !== last) { last = n; chrome.runtime.sendMessage({ type: "count", n, top: window === window.top }); }
    } catch (e) { /* extensão recarregada */ }
  }
  report();
  new MutationObserver(() => { if (!timer) timer = setTimeout(report, 1200); })
    .observe(document.documentElement, { childList: true, subtree: true, attributes: true, attributeFilter: ["src"] });
})();
