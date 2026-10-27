/*! Lobby waitlist widget · MIT */
(function () {
  var s = document.currentScript || document.querySelector("script[data-project]");
  if (!s) return;
  var d = s.dataset, pk = d.project, api = new URL(s.src).origin, t0 = Date.now();
  var q = new URLSearchParams(location.search);
  var host = d.target ? document.querySelector(d.target) : null;
  if (!host) { host = document.createElement("div"); s.parentNode.insertBefore(host, s.nextSibling); }
  var root = host.attachShadow ? host.attachShadow({ mode: "open" }) : host;
  var css =
    ":host{display:block}*{box-sizing:border-box;font:inherit}" +
    "form{display:flex;gap:8px;flex-wrap:wrap}" +
    "input{flex:1 1 200px;min-width:0;padding:var(--wl-pad,11px 13px);border:1px solid var(--wl-border,#d4d3cf);border-radius:var(--wl-radius,10px);background:var(--wl-bg,#fff);color:var(--wl-ink,#17171a)}" +
    "button{padding:var(--wl-pad,11px 16px);border:0;border-radius:var(--wl-radius,10px);background:var(--wl-accent,#17171a);color:var(--wl-accent-ink,#fff);font-weight:600;cursor:pointer}" +
    "button[disabled]{opacity:.6}.hp{position:absolute;left:-9999px}[hidden]{display:none!important}" +
    "p{margin:10px 0 0;color:var(--wl-muted,#55545c);font-size:.92em}.err{color:var(--wl-error,#b3261e)}" +
    "b{color:var(--wl-ink,#17171a)}.row{display:flex;gap:8px;margin-top:8px}.row input{font-size:.9em}";
  root.innerHTML =
    "<style>" + css + "</style>" +
    '<form part="form"><input class="hp" name="website" tabindex="-1" autocomplete="off" aria-hidden="true">' +
    '<input part="input" type="email" name="email" required autocomplete="email" aria-label="Email address" placeholder="' + esc(d.placeholder || "you@example.com") + '">' +
    '<button part="button" type="submit">' + esc(d.button || "Join the waitlist") + "</button></form>" +
    '<p part="message" role="status" hidden></p>';
  var f = root.querySelector("form"), msg = root.querySelector("p"), btn = root.querySelector("button");

  if (d.counter != null) {
    fetch(api + "/v1/w/" + pk + "/count").then(function (r) { return r.json(); }).then(function (j) {
      if (j.count != null) show("<b>" + j.count.toLocaleString() + "</b> people are already waiting.");
    }).catch(function () {});
  }

  f.addEventListener("submit", function (e) {
    e.preventDefault();
    var body = {
      email: f.email.value, website: f.website.value, ref: q.get("ref") || "",
      source_url: location.href, referrer: document.referrer, _dwell: Date.now() - t0
    };
    ["source", "medium", "campaign", "term", "content"].forEach(function (k) { body["utm_" + k] = q.get("utm_" + k) || ""; });
    btn.disabled = true;
    fetch(api + "/v1/w/" + pk + "/signup", {
      method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify(body)
    }).then(function (r) { return r.json(); }).then(function (j) {
      btn.disabled = false;
      if (!j.ok) return show(esc(j.error || "Something went wrong."), 1);
      host.dispatchEvent(new CustomEvent("lobby:signup", { detail: j, bubbles: true }));
      if (j.confirm_required) { f.hidden = true; return show(esc(d.success || j.message)); }
      f.hidden = true;
      show((j.position ? "You're <b>#" + j.position + "</b> in line. " : "You're on the list. ") +
        "Share your link to move up:" + '<span class="row"><input readonly value="' + esc(j.referral_link) + '"><button type="button">Copy</button></span>');
      var c = root.querySelector(".row button"), i = root.querySelector(".row input");
      c.onclick = function () { navigator.clipboard && navigator.clipboard.writeText(i.value); c.textContent = "Copied"; };
    }).catch(function () { btn.disabled = false; show("Network error. Please try again.", 1); });
  });

  function show(h, err) { msg.hidden = false; msg.className = err ? "err" : ""; msg.innerHTML = h; }
  function esc(v) { return String(v).replace(/[&<>"']/g, function (c) { return "&#" + c.charCodeAt(0) + ";"; }); }
})();
