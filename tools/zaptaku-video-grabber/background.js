// Badge com o número de vídeos encontrados por aba (somando os frames).
const counts = {}; // tabId -> {frameId: n}
function paint(tabId) {
  const total = Object.values(counts[tabId] || {}).reduce((a, b) => a + b, 0);
  chrome.action.setBadgeBackgroundColor({ tabId, color: "#FF7A1A" });
  chrome.action.setBadgeText({ tabId, text: total ? String(total) : "" });
}
chrome.runtime.onMessage.addListener((msg, sender) => {
  if (msg.type !== "count" || !sender.tab) return;
  const t = sender.tab.id;
  (counts[t] = counts[t] || {})[sender.frameId || 0] = msg.n;
  paint(t);
});
chrome.tabs.onUpdated.addListener((tabId, info) => { if (info.status === "loading") { delete counts[tabId]; paint(tabId); } });
chrome.tabs.onRemoved.addListener(tabId => { delete counts[tabId]; });
