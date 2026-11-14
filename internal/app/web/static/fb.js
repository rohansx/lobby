/*! Lobby feedback widget · MIT */
(function () {
  var s = document.currentScript || document.querySelector("script[data-reporter]");
  if (!s || window.__lobbyFeedback) return;
  window.__lobbyFeedback = 1;
  var d = s.dataset, api = new URL(s.src).origin, errs = [], reqs = [];

  // Context the report carries: recent console errors and failed requests.
  // Only messages, paths and status codes; never bodies, headers or query strings.
  function push(list, v, max) { list.push(v); if (list.length > max) list.shift(); }
  function str(v) { try { return v instanceof Error ? v.name + ": " + v.message : typeof v == "string" ? v : JSON.stringify(v); } catch (e) { return String(v); } }
  var oe = console.error;
  console.error = function () { push(errs, [].map.call(arguments, str).join(" ").slice(0, 300), 20); return oe.apply(console, arguments); };
  addEventListener("error", function (e) { if (e.message) push(errs, (e.message + (e.filename ? " (" + e.filename.split("?")[0] + ":" + e.lineno + ")" : "")).slice(0, 300), 20); });
  addEventListener("unhandledrejection", function (e) { push(errs, ("Unhandled rejection: " + str(e.reason)).slice(0, 300), 20); });
  var capturing = 0;
  function failed(u, status) {
    if (capturing) return; // the screenshot's own font and image fetches aren't the app's
    try { var x = new URL(u, location.href); if (x.origin !== api) push(reqs, { path: x.pathname, status: status }, 10); } catch (e) {}
  }
  if (window.fetch) {
    var of = window.fetch;
    window.fetch = function (i) {
      var u = typeof i == "string" ? i : (i && i.url) || String(i);
      return of.apply(this, arguments).then(function (r) { if (!r.ok) failed(u, r.status); return r; }, function (e) { failed(u, 0); throw e; });
    };
  }
  var xo = XMLHttpRequest.prototype.open, xs = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open = function (m, u) { this.__lfu = u; return xo.apply(this, arguments); };
  XMLHttpRequest.prototype.send = function () {
    var x = this;
    x.addEventListener("loadend", function () { if (x.status === 0 || x.status >= 400) failed(x.__lfu, x.status); });
    return xs.apply(this, arguments);
  };

  if (!d.reporter || !d.project) return; // only invited users get the button

  var host = document.createElement("div");
  host.setAttribute("data-lobby-mask", "");
  // Brand colours: data-accent / data-accent-ink, or --wl-* variables on :root.
  if (d.accent) host.style.setProperty("--wl-accent", d.accent);
  if (d.accentInk) host.style.setProperty("--wl-accent-ink", d.accentInk);
  var side = d.position === "left" ? "left" : "right";
  var root = host.attachShadow({ mode: "open" });

  var icon = {
    chat: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M21 12a8 8 0 0 1-11.6 7.1L4 20l1-4.6A8 8 0 1 1 21 12Z"/></svg>',
    bug: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="7" y="8" width="10" height="12" rx="5"/><path d="M12 8V5m-3 0 1.5 3M15 5l-1.5 3M3 13h4m10 0h4M4 19l3-2m13 2-3-2M4 7l3 2m13-2-3 2"/></svg>',
    confusing: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.6.3-1 .9-1 1.6V14m0 3h.01"/></svg>',
    idea: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 18h6m-5 3h4M12 3a6 6 0 0 0-3.5 10.9c.6.4 1 1.2 1 2V16h5v-.1c0-.8.4-1.6 1-2A6 6 0 0 0 12 3Z"/></svg>',
    check: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12.5 4.5 4.5L19 7.5"/></svg>',
    x: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg>',
    mic: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="9" y="3" width="6" height="11" rx="3"/><path d="M5 11a7 7 0 0 0 14 0M12 18v3"/></svg>'
  };
  var hints = {
    bug: "What happened, and what did you expect?",
    confusing: "What was unclear, and what were you trying to do?",
    idea: "What would you like this to do?"
  };

  var css =
    ":host{all:initial;--a:var(--wl-accent,#18181b);--ai:var(--wl-accent-ink,#fff);--bg:var(--wl-bg,#fff);--ink:var(--wl-ink,#18181b);" +
    "--mut:var(--wl-muted,#71717a);--line:var(--wl-border,#e4e4e7);--soft:var(--wl-soft,#f4f4f5);--r:var(--wl-radius,12px);--err:var(--wl-error,#b3261e)}" +
    "@media (prefers-color-scheme:dark){:host{--a:var(--wl-accent,#fafafa);--ai:var(--wl-accent-ink,#18181b);--bg:var(--wl-bg,#1c1c1f);--ink:var(--wl-ink,#fafafa);" +
    "--mut:var(--wl-muted,#a1a1aa);--line:var(--wl-border,#3f3f46);--soft:var(--wl-soft,#27272a)}}" +
    "*{box-sizing:border-box;font:14px/1.45 system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;letter-spacing:normal}" +
    "svg{width:16px;height:16px;fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round;stroke-linejoin:round;flex:none}" +
    "button{cursor:pointer;border:0;background:none;color:inherit}button:focus-visible,textarea:focus-visible{outline:2px solid var(--a);outline-offset:2px}" +
    ".fab{position:fixed;" + side + ":20px;bottom:20px;z-index:2147483646;display:flex;align-items:center;gap:7px;padding:10px 15px 10px 13px;border-radius:999px;" +
    "background:var(--a);color:var(--ai);font-weight:600;box-shadow:0 6px 20px rgba(0,0,0,.18),0 1px 3px rgba(0,0,0,.12);transition:transform .15s,box-shadow .15s}" +
    ".fab:hover{transform:translateY(-1px);box-shadow:0 10px 26px rgba(0,0,0,.22),0 1px 3px rgba(0,0,0,.12)}" +
    ".box{position:fixed;z-index:2147483645;pointer-events:none;border:2px solid var(--a);border-radius:6px;background:color-mix(in srgb,var(--a) 10%,transparent);" +
    "box-shadow:0 0 0 4000px rgba(0,0,0,.06);display:none;transition:all .08s ease-out}" +
    ".tag{position:absolute;left:-2px;bottom:calc(100% + 6px);max-width:280px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;padding:3px 8px;border-radius:6px;" +
    "background:var(--a);color:var(--ai);font:600 12px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace}.box.low .tag{bottom:auto;top:calc(100% + 6px)}" +
    ".bar{position:fixed;left:50%;top:16px;transform:translateX(-50%);z-index:2147483647;display:flex;align-items:center;gap:10px;padding:7px 8px 7px 16px;border-radius:999px;" +
    "background:var(--bg);color:var(--ink);border:1px solid var(--line);box-shadow:0 8px 30px rgba(0,0,0,.16);max-width:calc(100vw - 24px)}" +
    ".bar span{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.bar kbd{font:600 11px/1 ui-monospace,monospace;padding:3px 5px;border:1px solid var(--line);border-radius:4px;color:var(--mut)}" +
    ".pill{padding:6px 11px;border-radius:999px;background:var(--soft);font-weight:500;white-space:nowrap}.pill:hover{background:var(--line)}" +
    ".panel{position:fixed;z-index:2147483647;width:340px;max-width:calc(100vw - 24px);background:var(--bg);color:var(--ink);border:1px solid var(--line);border-radius:var(--r);" +
    "padding:14px;box-shadow:0 16px 48px rgba(0,0,0,.2),0 2px 6px rgba(0,0,0,.08);animation:in .16s ease-out}" +
    "@keyframes in{from{opacity:0;transform:translateY(4px) scale(.98)}}" +
    ".head{display:flex;align-items:center;justify-content:space-between;margin-bottom:10px}.head b{font-weight:600;font-size:15px}" +
    ".icon{display:grid;place-items:center;width:28px;height:28px;border-radius:8px;color:var(--mut)}.icon:hover{background:var(--soft);color:var(--ink)}" +
    ".chip{display:flex;align-items:center;gap:8px;margin-bottom:10px;padding:6px 6px 6px 10px;border-radius:8px;background:var(--soft);font-size:12.5px;color:var(--mut)}" +
    ".chip code{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:12px ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--ink)}" +
    ".chip button{font-size:12.5px;font-weight:600;padding:3px 8px;border-radius:6px;color:var(--ink)}.chip button:hover{background:var(--line)}" +
    ".types{display:grid;grid-template-columns:repeat(3,1fr);gap:4px;padding:3px;margin-bottom:10px;border-radius:10px;background:var(--soft)}" +
    ".types button{display:flex;align-items:center;justify-content:center;gap:6px;padding:7px 4px;border-radius:8px;color:var(--mut);font-weight:500}" +
    ".types button[aria-checked=true]{background:var(--bg);color:var(--ink);box-shadow:0 1px 3px rgba(0,0,0,.12)}" +
    "textarea{display:block;width:100%;min-height:92px;max-height:240px;resize:vertical;padding:9px 11px;border:1px solid var(--line);border-radius:10px;background:var(--bg);color:var(--ink)}" +
    "textarea::placeholder{color:var(--mut)}" +
    ".foot{display:flex;align-items:center;gap:8px;margin-top:10px}.n{margin-right:auto;color:var(--mut);font-size:12px}" +
    ".send{display:flex;align-items:center;gap:6px;padding:8px 14px;border-radius:9px;background:var(--a);color:var(--ai);font-weight:600}.send[disabled]{opacity:.55;cursor:default}" +
    ".send kbd{font:11px/1 system-ui;opacity:.7}.fine{margin:10px 0 0;font-size:12px;color:var(--mut)}" +
    ".msg{margin:10px 0 0;font-size:13px;color:var(--err)}" +
    ".shot{margin-bottom:10px;border:1px solid var(--line);border-radius:8px;overflow:hidden;background:var(--soft)}" +
    ".shot img{display:block;width:100%;max-height:150px;object-fit:cover;object-position:top}.shot .wait{padding:14px 10px;font-size:12.5px;color:var(--mut)}" +
    ".shotbar{display:flex;align-items:center;justify-content:space-between;gap:8px;padding:5px 6px 5px 10px;font-size:12px;color:var(--mut);border-top:1px solid var(--line)}" +
    ".shotbar button{font-size:12px;font-weight:600;padding:3px 8px;border-radius:6px;color:var(--ink)}.shotbar button:hover{background:var(--line)}" +
    ".mic{display:flex;align-items:center;gap:6px;padding:7px 10px;border-radius:9px;background:var(--soft);font-weight:500;font-size:13px}.mic:hover{background:var(--line)}" +
    ".mic.rec{background:var(--err);color:#fff}.mic.rec svg{animation:pulse 1s ease-in-out infinite}.mic[disabled]{opacity:.6;cursor:default}" +
    "@keyframes pulse{50%{opacity:.35}}" +
    ".fu{width:100%;text-align:left;margin-top:8px}.fu p{margin:0 0 8px;color:var(--ink);font-weight:500}.fu textarea{min-height:70px}" +
    ".fu .foot{justify-content:flex-end}.fu .foot .n{display:none}" +
    ".done{display:flex;flex-direction:column;align-items:center;text-align:center;gap:6px;padding:18px 8px 10px}" +
    ".done i{display:grid;place-items:center;width:40px;height:40px;border-radius:50%;background:var(--a);color:var(--ai)}.done i svg{width:20px;height:20px;stroke-width:2.5}" +
    ".done b{font-weight:600;font-size:15px}.done p{margin:0;color:var(--mut)}" +
    ".spin{display:block;width:18px;height:18px;border:2.5px solid currentColor;border-right-color:transparent;border-radius:50%;animation:rot .7s linear infinite}" +
    "@keyframes rot{to{transform:rotate(360deg)}}" +
    "[hidden]{display:none!important}.sr{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0)}" +
    "@media (max-width:560px){.panel{left:12px!important;right:12px!important;top:auto!important;bottom:12px!important;width:auto;max-width:none}.bar kbd,.hide-sm{display:none}}" +
    "@media (prefers-reduced-motion:reduce){*{animation:none!important;transition:none!important}}";

  root.innerHTML =
    "<style>" + css + "</style>" +
    '<button class="fab" type="button" part="button" aria-haspopup="dialog">' + icon.chat + "<span>" + esc(d.button || "Feedback") + "</span></button>" +
    '<div class="box"><span class="tag"></span></div>' +
    '<div class="bar" hidden><span>Click what\'s wrong<span class="hide-sm">, or Tab to it and press <kbd>Enter</kbd></span></span>' +
    '<button class="pill" type="button" data-page>Whole page</button><button class="pill" type="button" data-cancel>Cancel <kbd>Esc</kbd></button></div>' +
    '<form class="panel" role="dialog" aria-label="Send feedback" hidden novalidate>' +
    '<div class="body"><div class="head"><b>Send feedback</b><button class="icon" type="button" data-cancel aria-label="Close">' + icon.x + "</button></div>" +
    '<div class="chip"><code></code><button type="button" data-repick>Change</button></div>' +
    '<div class="shot" hidden><div class="wait">Taking a screenshot of this area…</div><img alt="Screenshot of the area you picked" hidden>' +
    '<div class="shotbar"><span>Screenshot · private parts blanked</span><button type="button" data-noshot>Remove</button></div></div>' +
    '<div class="types" role="radiogroup" aria-label="What kind of feedback">' +
    '<button type="button" role="radio" data-type="bug" aria-checked="true">' + icon.bug + "Bug</button>" +
    '<button type="button" role="radio" data-type="confusing" aria-checked="false">' + icon.confusing + "Confusing</button>" +
    '<button type="button" role="radio" data-type="idea" aria-checked="false">' + icon.idea + "Idea</button></div>" +
    '<textarea name="comment" maxlength="2000" aria-label="Your feedback"></textarea>' +
    '<div class="foot"><button class="mic" type="button" hidden aria-label="Speak instead of typing">' + icon.mic + '<span>Speak</span></button><span class="n">0 / 2000</span><button class="send" type="submit" disabled>Send <kbd class="hide-sm">' + (/Mac|iP/.test(navigator.platform) ? "⌘" : "Ctrl") + " ↵</kbd></button></div>" +
    '<p class="msg" role="alert" hidden></p>' +
    '<p class="fine"></p></div>' +
    '<div class="done" hidden><i>' + icon.check + '</i><b>Thanks, got it</b><p></p>' +
    '<div class="fu" hidden><p></p><textarea name="answer" maxlength="1000" aria-label="Your answer"></textarea>' +
    '<div class="foot"><button class="pill" type="button" data-skip>Skip</button><button class="send" type="button" data-answer disabled>Send</button></div></div></div></form>' +
    '<span class="sr" aria-live="polite"></span>';

  function $(q) { return root.querySelector(q); }
  var fab = $(".fab"), box = $(".box"), tag = $(".tag"), bar = $(".bar"), form = $("form"), body = $(".body"), done = $(".done"),
    msg = $(".msg"), count = $(".n"), send = $(".send"), ta = form.comment, live = $(".sr"),
    picking = false, picked = null, type = "bug", shots = false, ask = false, sentMsg = "", shot = null, shotURL = "", round = 0,
    mic = $(".mic"), rec = null,
    shotBox = $(".shot"), shotImg = $(".shot img"), fu = $(".fu"), fuText = fu.querySelector("textarea"), fuSend = $("[data-answer]");

  function mount() { if (!host.isConnected) document.documentElement.appendChild(host); }
  // Ask Lobby first: the founder can switch the widget off for someone at any
  // time, and it should disappear without the app changing anything.
  of0(api + "/v1/feedback/access?project=" + encodeURIComponent(d.project) + "&reporter=" + encodeURIComponent(d.reporter))
    .then(function (r) { return r.json(); })
    .then(function (j) {
      if (!j.enabled) return;
      shots = !!j.screenshots; ask = !!j.ask;
      var talk = j.voice && micAllowed();
      mic.hidden = !talk;
      $(".fine").textContent = "Sends the page path, this element" + (shots ? ", a screenshot of this area with private parts blanked" : "") +
        " and recent errors. Never what you typed into the page." + (talk ? " Voice is transcribed by Sarvam AI and not kept." : "");
      mount();
      // Single-page apps sometimes replace the whole document body; stay attached.
      new MutationObserver(mount).observe(document.documentElement, { childList: true });
    }).catch(function () {});

  function ours(el) { return el === host || host.contains(el); }
  function label(el) {
    if (el === document.body || el === document.documentElement) return "The whole page";
    var tag = el.tagName.toLowerCase(), t = textOf(el);
    var name = { a: "link", button: "button", img: "image", input: "field", select: "dropdown", textarea: "text box", tr: "row", li: "item", td: "cell", th: "column heading" }[tag] || tag;
    if (t) return name + " · " + t.slice(0, 48);
    var h = hrefOf(el);
    if (h) return (name === "link" ? "" : name + " · ") + "link to " + h;
    if (el.closest("[data-lobby-mask]")) return name + " · private text";
    var c = contextOf(el).split(" › ").pop();
    return name + (c ? " in " + c.slice(0, 48) : "");
  }
  function frame(el) {
    if (!el || ours(el) || el === document.body || el === document.documentElement) { box.style.display = "none"; return; }
    var r = el.getBoundingClientRect();
    box.style.cssText = "display:block;left:" + (r.left - 3) + "px;top:" + (r.top - 3) + "px;width:" + (r.width + 6) + "px;height:" + (r.height + 6) + "px";
    box.className = "box" + (r.top < 34 ? " low" : "");
    tag.textContent = label(el);
  }

  // Pick mode: swallow the page's own pointer events so picking doesn't act.
  var block = ["click", "mousedown", "mouseup", "pointerdown", "pointerup", "touchstart", "touchend", "dblclick", "contextmenu"];
  function stop(e) {
    if (!picking || ours(e.target)) return;
    e.preventDefault(); e.stopPropagation(); e.stopImmediatePropagation();
    // Cancelling touchend stops the browser making a click, so a tap picks here.
    if (e.type === "click" || e.type === "touchend") choose(e.target);
  }
  function over(e) { if (picking && !ours(e.target)) frame(e.target); }
  function key(e) {
    var open = !form.hidden;
    if (e.key === "Escape" && (picking || open)) { e.preventDefault(); e.stopPropagation(); return reset(); }
    if (picking && (e.key === "Enter" || e.key === " ") && document.activeElement && document.activeElement !== document.body && !ours(document.activeElement)) {
      e.preventDefault(); e.stopPropagation(); choose(document.activeElement);
    }
  }
  addEventListener("keydown", key, true);
  var cursor = document.createElement("style");
  cursor.textContent = "*{cursor:crosshair!important}";

  function start() {
    picking = true; fab.hidden = true; bar.hidden = false; form.hidden = true; box.style.display = "none"; tag.style.display = "";
    document.head.appendChild(cursor);
    // The button just hid itself; start Tab from the top of the page, not after the widget.
    var b = document.body, had = b.hasAttribute("tabindex");
    if (!had) b.setAttribute("tabindex", "-1");
    b.focus({ preventScroll: true });
    if (!had) b.removeAttribute("tabindex");
    live.textContent = "Pick mode. Click the part of the page that's wrong, or Tab to it and press Enter. Escape cancels.";
    block.forEach(function (t) { document.addEventListener(t, stop, true); });
    document.addEventListener("mouseover", over, true);
    document.addEventListener("focusin", over, true);
  }
  function endPick() {
    picking = false; bar.hidden = true;
    if (cursor.parentNode) cursor.parentNode.removeChild(cursor);
    block.forEach(function (t) { document.removeEventListener(t, stop, true); });
    document.removeEventListener("mouseover", over, true);
    document.removeEventListener("focusin", over, true);
  }
  function reset() {
    endPick(); picked = null; form.hidden = true; fab.hidden = false; box.style.display = "none"; round++;
    body.hidden = false; done.hidden = true; msg.hidden = true; ta.value = ""; sync(); fab.focus();
    shot = null; shotURL = ""; shotBox.hidden = true; fu.hidden = true; fuText.value = "";
    stopMic(true);
  }
  function choose(el) {
    endPick(); picked = el; frame(el);
    snap(el);
    tag.style.display = "none"; // the panel's chip says the same
    $(".chip code").textContent = label(el);
    body.hidden = false; done.hidden = true; msg.hidden = true;
    form.hidden = false; place(); ta.focus();
  }
  // Open the panel beside the picked element; phones get a bottom sheet (CSS).
  function place() {
    if (!picked || form.hidden) return;
    var w = form.offsetWidth, h = form.offsetHeight, W = innerWidth, H = innerHeight, g = 12, top, left;
    if (picked === document.body || picked === document.documentElement) {
      left = side === "left" ? 20 : W - w - 20; top = H - h - 20;
    } else {
      var r = picked.getBoundingClientRect();
      left = Math.min(Math.max(r.left, g), W - w - g);
      top = r.bottom + 10 + h <= H - g ? r.bottom + 10 : r.top - 10 - h >= g ? r.top - 10 - h : H - h - g;
      if (top === H - h - g && r.right + 10 + w <= W - g) { left = r.right + 10; top = Math.min(Math.max(r.top, g), H - h - g); }
    }
    form.style.left = left + "px"; form.style.top = Math.max(top, g) + "px";
  }
  function setType(t) {
    type = t;
    [].forEach.call(root.querySelectorAll("[data-type]"), function (b) { b.setAttribute("aria-checked", String(b.dataset.type === t)); });
    ta.placeholder = hints[t];
  }
  function sync() { count.textContent = ta.value.length + " / 2000"; send.disabled = !ta.value.trim(); }

  fab.onclick = start;
  [].forEach.call(root.querySelectorAll("[data-cancel]"), function (b) { b.onclick = reset; });
  $("[data-page]").onclick = function () { choose(document.body); };
  $("[data-repick]").onclick = function () { box.style.display = "none"; start(); };
  [].forEach.call(root.querySelectorAll("[data-type]"), function (b) { b.onclick = function () { setType(b.dataset.type); ta.focus(); }; });
  $(".types").onkeydown = function (e) {
    var order = ["bug", "confusing", "idea"], i = order.indexOf(type);
    if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
      e.preventDefault(); setType(order[(i + (e.key === "ArrowRight" ? 1 : 2)) % 3]); $('[data-type="' + type + '"]').focus();
    }
  };
  ta.oninput = sync;
  ta.onkeydown = function (e) { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); form.requestSubmit ? form.requestSubmit() : form.onsubmit(e); } };
  addEventListener("scroll", function () { if (picked && !form.hidden && body.hidden === false) { frame(picked); place(); } }, true);
  addEventListener("resize", place);
  setType("bug"); sync();

  form.onsubmit = function (e) {
    e.preventDefault();
    if (!picked || !ta.value.trim() || send.disabled) return;
    send.disabled = true;
    var pending = shot ? Promise.race([shot, new Promise(function (ok) { setTimeout(function () { ok(""); }, 5000); })]) : Promise.resolve("");
    pending.then(function () { submit(); });
  };
  function submit() {
    var whole = picked === document.body || picked === document.documentElement, mine = round;
    var payload = {
      project: d.project, reporter: d.reporter, release: d.release || "",
      type: type, comment: ta.value, url: location.pathname,
      selector: whole ? "body" : selector(picked), tag: whole ? "body" : picked.tagName.toLowerCase(), text: whole ? "" : textOf(picked), context: whole ? "" : contextOf(picked),
      href: whole ? "" : hrefOf(picked), outline: whole ? "" : outlineOf(picked),
      viewport: innerWidth + "x" + innerHeight, browser: navigator.userAgent,
      console_errors: errs.slice(), failed_requests: reqs.slice(), screenshot: shotURL
    };
    of0(api + "/v1/feedback", {
      method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify(payload)
    }).then(function (r) { return r.json(); }).then(function (j) {
      if (!j.ok) { sync(); return note(j.error || "Something went wrong. Please try again."); }
      body.hidden = true; done.hidden = false; box.style.display = "none";
      sentMsg = j.message || "Thanks for the feedback.";
      ta.value = ""; sync();
      followup(j.report, mine);
    }).catch(function () { sync(); note("Couldn't reach the server. Check your connection and try again."); });
  }
  // ---- Screenshot of the picked area ----
  // Measured the moment someone picks, rendered from the DOM (no screen-share
  // prompt), then every data-lobby-mask region and every typed-in field is painted
  // over before the image exists anywhere but this page. They see it and can remove it.
  var lib = null, fields = "textarea,select,[contenteditable]:not([contenteditable=false])," +
    "input:not([type=checkbox]):not([type=radio]):not([type=button]):not([type=submit]):not([type=reset]):not([type=hidden]):not([type=range]):not([type=color]):not([type=file]):not([type=image])";
  function loadShot() {
    return lib || (lib = new Promise(function (ok, no) {
      if (window.__lobbyShot) return ok(window.__lobbyShot);
      var sc = document.createElement("script");
      sc.src = api + "/v1/fb-shot.js"; sc.async = true;
      sc.onload = function () { window.__lobbyShot ? ok(window.__lobbyShot) : no(); };
      sc.onerror = function () { lib = null; no(); };
      document.head.appendChild(sc);
    }));
  }
  function snap(el) {
    shot = null; shotURL = ""; shotImg.hidden = true; $(".shot .wait").hidden = false;
    shotBox.hidden = !shots;
    if (!shots) return;
    var mine = ++round, take = capture(el); // a newer pick or Remove makes this one stale
    shot = take.then(function (url) { if (mine === round) { shotURL = url; shotImg.src = url; shotImg.hidden = false; $(".shot .wait").hidden = true; } return url; },
      function () { if (mine === round) shotBox.hidden = true; return ""; });
  }
  // In the copy that gets rendered: hidden parts and fields become solid blocks
  // (exactly where they are, whatever the layout), and the picked element gets a marker.
  var fieldTags = /^(INPUT|TEXTAREA|SELECT)$/;
  function blank(n) {
    if (n.nodeType !== 1) return;
    if (n.hasAttribute("data-lobby-picked")) {
      n.removeAttribute("data-lobby-picked");
      n.style.setProperty("outline", "2px solid rgb(255, 0, 255)", "important");
      n.style.setProperty("outline-offset", "3px", "important");
    }
    var field = fieldTags.test(n.tagName) && !/^(checkbox|radio|button|submit|reset|hidden|range|color|file|image)$/i.test(n.getAttribute("type") || "");
    var editable = n.hasAttribute("contenteditable") && n.getAttribute("contenteditable") !== "false";
    if (!field && !editable) {
      if (n.hasAttribute("data-lobby-mask")) bars(n);
      return;
    }
    n.style.setProperty("background", "#9ca3af", "important");
    n.style.setProperty("color", "transparent", "important");
    n.style.setProperty("-webkit-text-fill-color", "transparent", "important");
    n.style.setProperty("text-shadow", "none", "important");
    n.style.setProperty("border-color", "#9ca3af", "important");
    if (field) { n.setAttribute("value", ""); n.removeAttribute("placeholder"); n.textContent = ""; try { n.value = ""; } catch (e) {} }
    [].forEach.call(n.querySelectorAll("*"), function (c) { if (c.style) c.style.setProperty("visibility", "hidden", "important"); });
  }
  // Private text becomes grey bars the size of each word, so the layout, colours,
  // badges and icons still show what the element is; pictures inside are hidden.
  function bars(root) {
    var w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT), list = [];
    while (w.nextNode()) list.push(w.currentNode);
    list.forEach(function (t) {
      var p = t.parentNode;
      if (!p || !/\S/.test(t.nodeValue)) return;
      if (p.namespaceURI === "http://www.w3.org/2000/svg") { if (p.style) p.style.setProperty("visibility", "hidden", "important"); return; }
      if (p.hasAttribute && p.hasAttribute("data-lobby-bar")) return; // already a bar (nested masks)
      var b = document.createElement("span");
      b.setAttribute("data-lobby-bar", "");
      b.textContent = t.nodeValue;
      // -webkit-text-fill-color too: the copy carries each element's computed fill colour, which beats color.
      b.style.cssText = "background:#c3c7ce!important;color:transparent!important;-webkit-text-fill-color:transparent!important;text-shadow:none!important;" +
        "border-radius:3px;-webkit-box-decoration-break:clone;box-decoration-break:clone";
      p.replaceChild(b, t);
    });
    [root].concat([].slice.call(root.querySelectorAll("*"))).forEach(function (m) {
      if (!m.style) return;
      m.style.setProperty("background-image", "none", "important");
      if (/^(IMG|PICTURE|VIDEO|CANVAS|IFRAME|OBJECT|EMBED)$/.test(m.tagName)) m.style.setProperty("visibility", "hidden", "important");
    });
  }
  function capture(el) {
    var whole = el === document.body || el === document.documentElement, W = innerWidth, H = innerHeight;
    var r = whole ? { left: 0, top: 0, right: W, bottom: H, width: W, height: H } : el.getBoundingClientRect(), pad = whole ? 0 : 56;
    var area = { left: Math.max(0, r.left - pad), top: Math.max(0, r.top - pad), right: Math.min(W, r.right + pad), bottom: Math.min(H, r.bottom + pad) };
    if (area.right - area.left < 4 || area.bottom - area.top < 4) return Promise.reject();
    // Render the smallest ancestor that holds the whole area, not the whole page.
    var root = whole ? document.body : el;
    while (root !== document.body && root.parentElement) {
      var b = root.getBoundingClientRect();
      if (b.left <= area.left && b.top <= area.top && b.right >= area.right && b.bottom >= area.bottom) break;
      root = root.parentElement;
    }
    var rr = root.getBoundingClientRect(), scale = Math.min(window.devicePixelRatio || 1, 2), masks = [];
    [].forEach.call(document.querySelectorAll(fields), function (n) {
      if (ours(n)) return;
      var b = n.getBoundingClientRect();
      if (b.width && b.height && b.right > area.left && b.left < area.right && b.bottom > area.top && b.top < area.bottom) masks.push(b);
    });
    var bg = getComputedStyle(document.body).backgroundColor;
    if (/rgba\(0, 0, 0, 0\)|transparent/.test(bg)) bg = getComputedStyle(document.documentElement).backgroundColor;
    if (/rgba\(0, 0, 0, 0\)|transparent/.test(bg)) bg = "#ffffff";
    var mark = getComputedStyle(host).getPropertyValue("--wl-accent").trim() || "#e11d48";
    if (!whole) el.setAttribute("data-lobby-picked", ""); // invisible; lets the copy find it
    return loadShot().then(function (m) {
      capturing++;
      return m.domToCanvas(root, { scale: scale, backgroundColor: bg, timeout: 4000, filter: function (n) { return n !== host; }, onCloneEachNode: blank })
        .finally(function () { capturing--; el.removeAttribute("data-lobby-picked"); });
    }, function (e) { el.removeAttribute("data-lobby-picked"); throw e; }).then(function (full) {
      // The copy can lay out a few pixels off the page (margins, defaults); find the
      // picked element's marker in the render to learn the shift.
      var dx = 0, dy = 0;
      if (!whole && r.top >= area.top && r.left >= area.left) {
        var ex = Math.round((r.left - rr.left - 5) * scale), ey = Math.round((r.top - rr.top - 5) * scale), R = Math.round(48 * scale);
        var x0 = Math.max(0, ex - R), y0 = Math.max(0, ey - R), ww = Math.min(full.width - x0, 2 * R), hh = Math.min(full.height - y0, 2 * R);
        if (ww > 0 && hh > 0) {
          var px = full.getContext("2d").getImageData(x0, y0, ww, hh).data, mx = 1e9, my = 1e9;
          for (var i = 0; i < px.length; i += 4) if (px[i] > 250 && px[i + 1] < 6 && px[i + 2] > 249) { var j = i / 4; mx = Math.min(mx, j % ww); my = Math.min(my, Math.floor(j / ww)); }
          if (mx < 1e9) { dx = x0 + mx - ex; dy = y0 + my - ey; }
        }
      }
      var w = Math.round((area.right - area.left) * scale), h = Math.round((area.bottom - area.top) * scale);
      var k = Math.min(1, 1400 / w), c = document.createElement("canvas");
      c.width = Math.round(w * k); c.height = Math.round(h * k);
      var g = c.getContext("2d"), s = scale * k;
      g.fillStyle = bg; g.fillRect(0, 0, c.width, c.height); // past the rendered element: page background, not black
      g.drawImage(full, (area.left - rr.left) * scale + dx, (area.top - rr.top) * scale + dy, w, h, 0, 0, c.width, c.height);
      // Second layer: paint over the same parts at their measured places.
      g.fillStyle = "#9ca3af";
      masks.forEach(function (b) { g.fillRect((b.left - area.left) * s, (b.top - area.top) * s, b.width * s, b.height * s); });
      if (!whole) { // outline what they picked, over the marker
        g.strokeStyle = mark; g.lineWidth = Math.max(4, 4 * s);
        g.strokeRect((r.left - area.left - 4) * s, (r.top - area.top - 4) * s, (r.width + 8) * s, (r.height + 8) * s);
      }
      for (var q = 0.82, url = ""; q > 0.3; q -= 0.2) {
        url = c.toDataURL("image/jpeg", q);
        if (url.length < 760000) return url;
      }
      throw new Error("screenshot too large");
    });
  }

  // ---- Voice: speak instead of typing (up to 30 s), transcribed to English ----
  function micAllowed() {
    var fp = document.permissionsPolicy || document.featurePolicy; // a Permissions-Policy header can block the mic
    return !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia && window.MediaRecorder) &&
      (!fp || !fp.allowsFeature || fp.allowsFeature("microphone"));
  }
  function micLabel(t) { mic.querySelector("span").textContent = t; }
  function stopMic(discard) {
    if (!rec) return;
    rec.discard = discard; clearInterval(rec.tick);
    if (rec.r.state !== "inactive") rec.r.stop();
  }
  mic.onclick = function () {
    if (rec) return stopMic(false);
    var types = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4", "audio/ogg;codecs=opus"], type = "";
    for (var i = 0; i < types.length; i++) if (MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(types[i])) { type = types[i]; break; }
    mic.disabled = true;
    navigator.mediaDevices.getUserMedia({ audio: true }).then(function (stream) {
      var r = new MediaRecorder(stream, type ? { mimeType: type } : undefined), parts = [], t0 = Date.now(), mine = round;
      rec = { r: r, discard: false };
      r.ondataavailable = function (e) { if (e.data && e.data.size) parts.push(e.data); };
      r.onstop = function () {
        stream.getTracks().forEach(function (t) { t.stop(); });
        var discard = rec.discard; rec = null; mic.classList.remove("rec");
        if (discard || mine !== round || !parts.length) { micLabel("Speak"); mic.disabled = false; return; }
        var blob = new Blob(parts, { type: (r.mimeType || type || "audio/webm") });
        mic.disabled = true; micLabel("Transcribing…"); live.textContent = "Transcribing your recording.";
        of0(api + "/v1/feedback/voice", { method: "POST", headers: { "Content-Type": blob.type, "X-Lobby-Project": d.project, "X-Lobby-Reporter": d.reporter }, body: blob })
          .then(function (res) { return res.json(); })
          .then(function (j) {
            if (mine !== round) return;
            if (!j.ok) return note(j.error || "Couldn't transcribe that. Please type instead.");
            if (j.text) { ta.value = (ta.value.trim() ? ta.value.replace(/\s+$/, "") + " " : "") + j.text; sync(); }
            else note("Didn't catch anything. Try again a little closer to the mic.");
            ta.focus();
          }, function () { note("Couldn't reach the server. Please type instead."); })
          .then(function () { mic.disabled = false; micLabel("Speak"); });
      };
      r.start(1000);
      mic.disabled = false; mic.classList.add("rec"); micLabel("0:00 · Stop"); msg.hidden = true;
      live.textContent = "Recording. Speak now; press the button again to stop.";
      rec.tick = setInterval(function () {
        var s = Math.floor((Date.now() - t0) / 1000);
        micLabel("0:" + (s < 10 ? "0" : "") + s + " · Stop");
        if (s >= 30) stopMic(false);
      }, 250);
    }, function () {
      mic.disabled = false;
      note("The microphone is blocked. Allow it in your browser, or type instead.");
    });
  };

  // ---- One follow-up question from Claude, when the report is vague ----
  // The sent state: a spinner while Claude looks at the report, then either one
  // question or a single "Thanks, got it".
  function doneState(wait, title, text) {
    done.querySelector("i").innerHTML = wait ? '<span class="spin"></span>' : icon.check;
    done.querySelector("b").textContent = title;
    var p = done.querySelector("p");
    p.textContent = text || ""; p.hidden = !text;
    live.textContent = title + (text ? ". " + text : "");
  }
  function followup(rid, mine) {
    function thanks() {
      if (mine !== round) return;
      doneState(false, "Thanks, got it", sentMsg);
      setTimeout(function () { if (mine === round && fu.hidden) reset(); }, 2500);
    }
    if (!rid || !ask) return thanks();
    doneState(true, "Sent", "Checking if anything's missing…");
    var body = JSON.stringify({ project: d.project, reporter: d.reporter });
    Promise.race([
      of0(api + "/v1/feedback/reports/" + rid + "/question", { method: "POST", headers: { "Content-Type": "application/json" }, body: body }).then(function (r) { return r.json(); }),
      new Promise(function (ok) { setTimeout(function () { ok({}); }, 12000); })
    ]).then(function (j) {
        if (mine !== round) return;
        if (!j.question) return thanks();
        doneState(false, "Thanks! One quick question", "");
        fu.querySelector("p").textContent = j.question;
        fu.hidden = false; fuText.value = ""; fuSend.disabled = true; fuText.focus();
        live.textContent = "One quick question: " + j.question;
        fu.dataset.rid = rid;
      }, thanks);
  }
  function answer() {
    var a = fuText.value.trim(), mine = round;
    if (!a) return;
    fuSend.disabled = true;
    of0(api + "/v1/feedback/reports/" + fu.dataset.rid + "/answer", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ project: d.project, reporter: d.reporter, answer: a })
    }).then(function (r) { return r.json(); }).then(function (j) {
      if (mine !== round) return;
      fu.hidden = true;
      doneState(false, j.ok ? "Thanks, that helps" : "Couldn't send that", j.ok ? "" : (j.error || "Please try again."));
      setTimeout(function () { if (mine === round) reset(); }, 2500);
    }, function () { fuSend.disabled = false; });
  }
  fuText.oninput = function () { fuSend.disabled = !fuText.value.trim(); };
  fuText.onkeydown = function (e) { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); answer(); } };
  fuSend.onclick = answer;
  $("[data-skip]").onclick = reset;
  $("[data-noshot]").onclick = function () { round++; shot = null; shotURL = ""; shotBox.hidden = true; ta.focus(); };

  // The widget's own request bypasses the failed-request recorder.
  function of0() { return (of || window.fetch).apply(window, arguments); }
  function note(t) { msg.hidden = false; msg.textContent = t; live.textContent = t; }

  // Ids that frameworks make up at runtime (React useId, Base UI, Radix, Headless UI,
  // hashes): they change between page loads, so a selector built on one finds nothing.
  function generated(v) {
    return /^(:r|_r_|r:)|[«»:]|_r_|\d{3,}|^\d/.test(v) ||
      /^(base-ui|radix|headlessui|mui|rc|react-aria|ember|chakra|mantine|downshift|reach|floating-ui|tippy|popper)[-_:]/i.test(v) ||
      /(^|[^a-z])(?=[a-z]*\d)[a-z\d]{6,}/i.test(v); // a hash-like run mixing letters and digits
  }
  var anchors = ["data-lobby-id", "data-testid", "data-test-id", "data-test", "data-cy", "data-qa"];
  // A selector that prefers founder-set anchors, then test ids, then stable ids, then the tag path.
  function selector(el) {
    var parts = [], q = window.CSS && CSS.escape ? CSS.escape : function (v) { return v.replace(/["\\]/g, "\\$&"); };
    outer: for (var n = el; n && n.nodeType === 1 && n !== document.documentElement && parts.length < 8; n = n.parentElement) {
      for (var i = 0; i < anchors.length; i++) {
        var v = n.getAttribute(anchors[i]);
        if (v && (i === 0 || !generated(v))) { parts.unshift("[" + anchors[i] + '="' + v.replace(/["\\]/g, "\\$&") + '"]'); break outer; }
      }
      if (n.id && !generated(n.id)) { parts.unshift("#" + q(n.id)); break; }
      var t = n.tagName.toLowerCase(), p = n.parentElement;
      if (p) {
        var same = [].filter.call(p.children, function (c) { return c.tagName === n.tagName; });
        if (same.length > 1) t += ":nth-of-type(" + (same.indexOf(n) + 1) + ")";
      }
      parts.unshift(t);
    }
    return parts.join(" > ");
  }
  // Visible text only: never input values, and nothing inside data-lobby-mask.
  var skip = /^(INPUT|TEXTAREA|SELECT|OPTION|SCRIPT|STYLE|NOSCRIPT|SVG)$/;
  function textOf(el) {
    if (el.closest("[data-lobby-mask],[contenteditable]")) return "";
    if (/^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName)) return (el.getAttribute("aria-label") || el.getAttribute("placeholder") || el.getAttribute("name") || "").slice(0, 200);
    var out = "";
    (function walk(n) {
      for (var c = n.firstChild; c && out.length < 200; c = c.nextSibling) {
        if (c.nodeType === 3) out += " " + c.nodeValue;
        else if (c.nodeType === 1 && !skip.test(c.tagName.toUpperCase()) && !c.hasAttribute("data-lobby-mask") && !c.isContentEditable) walk(c);
      }
    })(el);
    out = out.replace(/\s+/g, " ").trim();
    return (out || el.getAttribute("aria-label") || el.getAttribute("title") || "").slice(0, 200);
  }
  // Where the element sits, outermost first: "Ledger check › Checks failing".
  // Built from headings and named regions (dialogs, tabs, sections, forms, tables),
  // which most apps and component libraries already mark up. Masked text stays out.
  var named = /^(SECTION|FORM|NAV|ASIDE|DIALOG|FIELDSET|TABLE|ARTICLE|MAIN)$/, heads = "h1,h2,h3,h4,h5,h6,[role=heading],legend,caption,summary";
  function byIds(ids) {
    return ids.split(/\s+/).map(function (id) { var x = document.getElementById(id); return x ? textOf(x) : ""; }).join(" ").trim();
  }
  function nameOf(n, from) {
    if (n.closest("[data-lobby-mask]")) return "";
    var role = n.getAttribute("role") || "", lb = n.getAttribute("aria-labelledby");
    if (named.test(n.tagName) || /^(dialog|alertdialog|tabpanel|region|group|form|navigation|complementary)$/.test(role)) {
      var al = n.getAttribute("aria-label");
      if (al) return al;
      if (lb) { var t = byIds(lb); if (t) return t; }
    }
    // A heading among the direct children, ahead of the branch the click came from: a card's or section's title.
    for (var c = n.firstElementChild; c && c !== from; c = c.nextElementSibling) {
      if (c.matches(heads)) return textOf(c);
      var h = c.matches("header,hgroup") ? c.querySelector(heads) : null;
      if (h) return textOf(h);
    }
    return "";
  }
  function contextOf(el) {
    var out = [], from = el, own = textOf(el);
    for (var n = el.parentElement; n && n !== document.documentElement && out.length < 4; from = n, n = n.parentElement) {
      var t = nameOf(n, from).slice(0, 60);
      if (t && out[0] !== t && t !== own) out.unshift(t);
    }
    var h1 = document.querySelector("main h1, h1");
    var page = h1 && !h1.contains(el) ? textOf(h1).slice(0, 60) : "";
    if (page && out.indexOf(page) < 0) out.unshift(page);
    return out.join(" › ").slice(0, 200);
  }
  // Where the element (or the link around it) goes: same-site path only.
  function hrefOf(el) {
    var a = el.closest && el.closest("a[href]");
    if (!a) return "";
    try { var u = new URL(a.getAttribute("href"), location.href); return u.origin === location.origin ? u.pathname : ""; } catch (e) { return ""; }
  }
  // Its shape for a coding agent: tags, roles and class names, never text.
  function tok(n) {
    var c = [].filter.call(n.classList || [], function (k) { return !/[:\[\]\/!@]/.test(k) && !generated(k); }).slice(0, 3);
    var r = n.getAttribute("role");
    return n.tagName.toLowerCase() + (c.length ? "." + c.join(".") : "") + (r ? "[role=" + r + "]" : "");
  }
  function outlineOf(el) {
    var kids = [].slice.call(el.children, 0, 6).map(tok);
    return (tok(el) + (kids.length ? " > " + kids.join(" + ") + (el.children.length > 6 ? " + …" : "") : "")).slice(0, 300);
  }
  function esc(v) { return String(v).replace(/[&<>"']/g, function (c) { return "&#" + c.charCodeAt(0) + ";"; }); }

  // "Open in app" from the dashboard: #lobby-fb=<selector> outlines the element.
  var m = /lobby-fb=([^&]+)/.exec(location.hash);
  if (m) setTimeout(function () {
    try {
      var el = document.querySelector(decodeURIComponent(m[1]));
      if (el) { el.scrollIntoView({ block: "center" }); frame(el); setTimeout(function () { box.style.display = "none"; }, 6000); }
    } catch (e) {}
  }, 600);
})();
