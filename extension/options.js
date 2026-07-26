// Options: local Daycore URL + import token, and custom Canvas domains (which
// need an optional host permission grant so background fetches carry cookies).

const t = (key, subs) => chrome.i18n.getMessage(key, subs) || key;

document.querySelectorAll("[data-i18n]").forEach((el) => {
  el.textContent = t(el.dataset.i18n);
});

const urlEl = document.getElementById("daycore-url");
const tokenEl = document.getElementById("import-token");
const domainsEl = document.getElementById("domains");
const savedEl = document.getElementById("saved");

chrome.storage.sync
  .get({ daycoreURL: "http://localhost:8080", importToken: "", customDomains: [] })
  .then(({ daycoreURL, importToken, customDomains }) => {
    urlEl.value = daycoreURL;
    tokenEl.value = importToken;
    domainsEl.value = customDomains.join("\n");
  });

document.getElementById("save").addEventListener("click", async () => {
  const domains = domainsEl.value
    .split("\n")
    .map((d) => d.trim().replace(/^https?:\/\//, "").replace(/\/.*$/, ""))
    .filter(Boolean);

  const denied = [];
  for (const host of domains) {
    const granted = await chrome.permissions
      .request({ origins: [`*://${host}/*`] })
      .catch(() => false);
    if (!granted) denied.push(host);
  }

  await chrome.storage.sync.set({
    daycoreURL: urlEl.value.trim() || "http://localhost:8080",
    importToken: tokenEl.value.trim(),
    customDomains: domains,
  });

  savedEl.textContent = denied.length ? t("optPermDenied", [denied.join(", ")]) : t("optSaved");
  savedEl.className = denied.length ? "error" : "";
});
