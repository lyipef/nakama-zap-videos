// Varre a página e devolve { files: [{url, name, kind, w, h, poster}], protectedCount }
// Só considera URLs http(s) diretas. Vídeos em blob:/MediaSource (streams, geralmente protegidos) são apenas contados.
(function () {
  if (window.__zaptakuScan) return;
  const EXT = /\.(mp4|webm|mov|m4v|mkv|ogv|ogg)(\?|#|$)/i;
  function abs(u) { try { return new URL(u, location.href).href; } catch (e) { return null; } }
  function nameFrom(u) {
    try {
      const p = new URL(u).pathname.split("/").filter(Boolean).pop() || "video";
      return decodeURIComponent(p);
    } catch (e) { return "video"; }
  }
  window.__zaptakuScan = function () {
    const out = new Map(); let protectedCount = 0;
    const add = (u, kind, extra) => {
      const a = abs(u);
      if (!a || !/^https?:/i.test(a)) return;
      if (/\.(m3u8|mpd)(\?|$)/i.test(a)) { protectedCount++; return; }  // playlists de streaming: não baixa
      if (!out.has(a)) out.set(a, Object.assign({ url: a, name: nameFrom(a), kind }, extra || {}));
    };
    document.querySelectorAll("video").forEach(v => {
      const extra = { w: v.videoWidth || 0, h: v.videoHeight || 0, poster: v.poster ? abs(v.poster) : "" };
      const srcs = [v.currentSrc, v.getAttribute("src")].concat([...v.querySelectorAll("source")].map(s => s.getAttribute("src")));
      let found = false;
      srcs.filter(Boolean).forEach(s => {
        if (/^(blob|mediasource):/i.test(s)) return;
        found = true; add(s, "video", extra);
      });
      if (!found && (v.currentSrc || "").startsWith("blob:")) protectedCount++;
    });
    document.querySelectorAll("a[href]").forEach(a => { if (EXT.test(a.getAttribute("href"))) add(a.getAttribute("href"), "link"); });
    document.querySelectorAll('meta[property="og:video"],meta[property="og:video:url"],meta[property="og:video:secure_url"],meta[name="twitter:player:stream"]')
      .forEach(m => { const c = m.getAttribute("content"); if (c && EXT.test(c)) add(c, "meta"); });
    return { files: [...out.values()], protectedCount };
  };
})();
