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
    mic: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="9" y="3" width="6" height="11" rx="3"/><path d="M5 11a7 7 0 0 0 14 0M12 18v3"/></svg>',
    info: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 11v5m0-8h.01"/></svg>'
  };
  var mac = /Mac|iP/.test(navigator.platform), shortcut = mac ? "⌥⇧F" : "Alt+Shift+F";
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
    ".hb{display:flex;gap:2px}.icon[aria-expanded=true]{background:var(--soft);color:var(--ink)}" +
    ".info{margin:-2px 0 10px;padding:8px 10px;border-radius:8px;background:var(--soft);font-size:12px;color:var(--mut);max-height:190px;overflow:auto}" +
    ".info dl{margin:0;display:grid;grid-template-columns:auto 1fr;gap:3px 10px}.info dt{font-weight:600;color:var(--ink)}" +
    ".info dd{margin:0;min-width:0;overflow-wrap:anywhere;font:11.5px/1.45 ui-monospace,SFMono-Regular,Menlo,monospace}.info p{margin:8px 0 0}" +
    ".offer{display:block;width:100%;padding:10px;font-size:12.5px;font-weight:600;color:var(--ink);text-align:left}.offer:hover{background:var(--line)}" +
    ".acts{display:flex;gap:6px;margin-top:8px}" +
    ".shot{margin-bottom:10px;border:1px solid var(--line);border-radius:8px;overflow:hidden;background:var(--soft)}" +
    ".shot img{display:block;width:100%;max-height:150px;object-fit:cover;object-position:top}.shot .wait{padding:14px 10px;font-size:12.5px;color:var(--mut)}" +
    ".shotbar{display:flex;align-items:center;justify-content:space-between;gap:8px;padding:5px 6px 5px 10px;font-size:12px;color:var(--mut);border-top:1px solid var(--line)}" +
    ".shotbar button{font-size:12px;font-weight:600;padding:3px 8px;border-radius:6px;color:var(--ink)}.shotbar button:hover{background:var(--line)}" +
    ".mic{display:flex;align-items:center;gap:6px;padding:7px 10px;border-radius:9px;background:var(--soft);font-weight:500;font-size:13px}.mic:hover{background:var(--line)}" +
    ".mic.rec{background:var(--err);color:#fff}.mic.rec.hear{box-shadow:0 0 0 3px color-mix(in srgb,var(--err) 35%,transparent)}.mic.rec svg{animation:pulse 1s ease-in-out infinite}.mic[disabled]{opacity:.6;cursor:default}" +
    "@keyframes pulse{50%{opacity:.35}}" +
    ".fu{width:100%;text-align:left;margin-top:8px}.fu p{margin:0 0 8px;color:var(--ink);font-weight:500}.fu textarea{min-height:70px}" +
    ".fu .foot{justify-content:flex-end}.fu .mic{margin-right:auto}.choices{display:flex;flex-wrap:wrap;gap:6px;margin-bottom:8px}.choices button{text-align:left}.fu .foot .n{display:none}" +
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
    '<button class="fab" type="button" part="button" aria-haspopup="dialog" title="' + shortcut + '">' + icon.chat + "<span>" + esc(d.button || "Feedback") + "</span></button>" +
    '<div class="box"><span class="tag"></span></div>' +
    '<div class="bar" hidden><span>Click or drag over what\'s wrong<span class="hide-sm">, <kbd>↑</kbd> <kbd>↓</kbd> to resize</span></span>' +
    '<button class="pill" type="button" data-page>Just write</button><button class="pill" type="button" data-cancel>Cancel <kbd>Esc</kbd></button></div>' +
    '<form class="panel" role="dialog" aria-label="Send feedback" hidden novalidate>' +
    '<div class="body"><div class="head"><b>Send feedback</b><span class="hb"><button class="icon" type="button" data-info aria-label="What gets sent" aria-expanded="false">' + icon.info + "</button>" +
    '<button class="icon" type="button" data-cancel aria-label="Close">' + icon.x + "</button></span></div>" +
    '<div class="info" hidden></div>' +
    '<div class="chip"><code></code><button type="button" data-repick>Change</button></div>' +
    '<div class="shot" hidden><div class="wait">Taking a screenshot of this area…</div><button class="offer" type="button" data-addshot hidden></button><img alt="Screenshot of the area you picked" hidden>' +
    '<div class="shotbar"><span>Screenshot of this area</span><button type="button" data-noshot>Remove</button></div></div>' +
    '<div class="types" role="radiogroup" aria-label="What kind of feedback">' +
    '<button type="button" role="radio" data-type="bug" aria-checked="true">' + icon.bug + "Bug</button>" +
    '<button type="button" role="radio" data-type="confusing" aria-checked="false">' + icon.confusing + "Confusing</button>" +
    '<button type="button" role="radio" data-type="idea" aria-checked="false">' + icon.idea + "Idea</button></div>" +
    '<textarea name="comment" maxlength="2000" aria-label="Your feedback"></textarea>' +
    '<div class="foot"><button class="mic" type="button" hidden aria-label="Speak instead of typing">' + icon.mic + '<span>Speak</span></button><span class="n">0 / 2000</span><button class="send" type="submit" disabled>Send <kbd class="hide-sm">' + (mac ? "⌘" : "Ctrl") + " ↵</kbd></button></div>" +
    '<p class="msg" role="alert" hidden></p></div>' +
    '<div class="done" hidden><i>' + icon.check + '</i><b>Thanks, got it</b><p></p>' +
    '<div class="fu" hidden><p></p><div class="choices"></div><textarea name="answer" maxlength="1000" aria-label="Your answer"></textarea>' +
    '<div class="foot"><button class="mic" type="button" hidden aria-label="Speak your answer">' + icon.mic + '<span>Speak</span></button><button class="pill" type="button" data-skip>Skip</button><button class="send" type="button" data-answer disabled>Send</button></div></div>' +
    '<div class="acts" hidden><button class="pill" type="button" data-again>Send another</button><button class="pill" type="button" data-cancel>Close</button></div></div></form>' +
    '<span class="sr" aria-live="polite"></span>';

  function $(q) { return root.querySelector(q); }
  var fab = $(".fab"), box = $(".box"), tag = $(".tag"), bar = $(".bar"), form = $("form"), body = $(".body"), done = $(".done"),
    msg = $(".msg"), count = $(".n"), send = $(".send"), ta = form.comment, live = $(".sr"),
    picking = false, picked = null, region = null, type = "bug", shots = false, ask = false, masks = false, sentMsg = "", shot = null, shotURL = "", round = 0, shotRound = 0,
    fine = "", talk = false, usedVoice = false, info = $(".info"), infoBtn = $("[data-info]"), acts = $(".acts"),
    shotBox = $(".shot"), shotImg = $(".shot img"), shotWait = $(".shot .wait"), shotBar = $(".shotbar"), offerBtn = $("[data-addshot]"),
    fu = $(".fu"), fuText = fu.querySelector("textarea"), fuSend = $("[data-answer]");

  function mount() { if (!host.isConnected) document.documentElement.appendChild(host); }
  // Ask Lobby first: the founder can switch the widget off for someone at any
  // time, and it should disappear without the app changing anything.
  of0(api + "/v1/feedback/access?project=" + encodeURIComponent(d.project) + "&reporter=" + encodeURIComponent(d.reporter))
    .then(function (r) { return r.json(); })
    .then(function (j) {
      if (!j.enabled) return;
      shots = !!j.screenshots; ask = !!j.ask; masks = !!j.masks;
      talk = j.voice && micAllowed();
      main.btn.hidden = fuMic.btn.hidden = !talk;
      fine = "Sends the page path, this element" + (shots ? ", a screenshot of this area" + (masks ? " with private parts blanked" : "") : "") +
        ", your last few clicks and recent errors. Never what you typed into the page." + (talk ? " Voice is transcribed by Sarvam AI and not kept." : "");
      mount();
      track();
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
    if (hidden(el)) return name + " · private text";
    var c = contextOf(el).split(" › ").pop();
    return name + (c ? " in " + c.slice(0, 48) : "");
  }
  function frame(el) {
    if (!el || ours(el) || el === document.body || el === document.documentElement) { box.style.display = "none"; return; }
    frameAt(el.getBoundingClientRect(), label(el));
  }
  function frameAt(r, text) {
    box.style.cssText = "display:block;left:" + (r.left - 3) + "px;top:" + (r.top - 3) + "px;width:" + (r.width + 6) + "px;height:" + (r.height + 6) + "px";
    box.className = "box" + (r.top < 34 ? " low" : "");
    tag.textContent = text;
  }
  function whole(el) { return el === document.body || el === document.documentElement; }

  // What a click most likely means: the button, not the icon or the span inside it.
  // Up and down arrows walk from there to bigger or smaller parts of the page.
  var act = "a,button,summary,label,input,select,textarea,[role=button],[role=tab],[role=menuitem],[role=option],[role=checkbox],[role=switch],[role=link],[onclick]";
  function smart(t) {
    if (!t || !t.closest) return t;
    var g = t.closest("svg");
    if (g && g.parentElement) t = g;
    return t.closest(act) || t;
  }
  var cur = null, smaller = [], down = null, dragged = false;

  // Pick mode: swallow the page's own pointer events so picking doesn't act.
  var block = ["click", "mousedown", "mouseup", "pointerdown", "pointerup", "touchstart", "touchend", "dblclick", "contextmenu"];
  function stop(e) {
    if (!picking || ours(e.target)) return;
    e.preventDefault(); e.stopPropagation(); e.stopImmediatePropagation();
    if (e.type === "pointerdown" && e.pointerType === "mouse" && e.button === 0) { down = { x: e.clientX, y: e.clientY }; dragged = false; return; }
    if (e.type === "pointerup" && dragged) return area(rectOf(down, e));
    if (e.type === "pointerup") down = null;
    // Cancelling touchend stops the browser making a click, so a tap picks here.
    if (e.type === "click" || e.type === "touchend") choose(cur && cur.contains(e.target) ? cur : smart(e.target));
    // Disabled buttons and fields get no click, only pointer events: pick them on pointerup.
    else if (e.type === "pointerup" && e.target.closest && e.target.closest(":disabled")) choose(e.target.closest(":disabled"));
  }
  function rectOf(a, e) {
    var l = Math.min(a.x, e.clientX), t = Math.min(a.y, e.clientY);
    return { left: l, top: t, width: Math.abs(e.clientX - a.x), height: Math.abs(e.clientY - a.y), right: Math.max(a.x, e.clientX), bottom: Math.max(a.y, e.clientY) };
  }
  // Dragging draws an area, for problems that aren't one element: a gap, a layout, a chart.
  function move(e) {
    if (!picking || !down) return;
    if (!dragged && Math.abs(e.clientX - down.x) + Math.abs(e.clientY - down.y) < 8) return;
    dragged = true;
    frameAt(rectOf(down, e), "Area");
  }
  function over(e) {
    if (!picking || ours(e.target) || dragged) return;
    var t = e.target;
    cur = smart(t); smaller = cur !== t ? [t] : [];
    frame(cur);
  }
  function key(e) {
    if (e.isTrusted === false) return; // our own Escape that closes a dropdown after picking from it
    var open = !form.hidden;
    if (e.altKey && e.shiftKey && e.code === "KeyF" && !picking && !open && host.isConnected) { e.preventDefault(); e.stopPropagation(); return start(); }
    if (e.key === "Escape" && (picking || open)) { e.preventDefault(); e.stopPropagation(); return reset(); }
    if (picking && cur && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
      e.preventDefault(); e.stopPropagation();
      var p = cur.parentElement;
      if (e.key === "ArrowUp" && p && !whole(p)) { smaller.push(cur); cur = p; }
      else if (e.key === "ArrowDown" && smaller.length) cur = smaller.pop();
      else if (e.key === "ArrowDown" && cur.firstElementChild) { smaller = []; cur = cur.firstElementChild; }
      return frame(cur);
    }
    if (picking && (e.key === "Enter" || e.key === " ") && document.activeElement && document.activeElement !== document.body && !ours(document.activeElement)) {
      e.preventDefault(); e.stopPropagation(); choose(cur && cur.contains(document.activeElement) ? cur : document.activeElement);
    }
  }
  addEventListener("keydown", key, true);
  var cursor = document.createElement("style");
  cursor.textContent = "*{cursor:crosshair!important}";

  // Opening the widget mustn't close what's open on the page: menus and popovers
  // close on a press or a focus change outside them. So the button takes no focus
  // and its press never reaches the page.
  ["pointerdown", "mousedown", "touchstart"].forEach(function (t) {
    fab.addEventListener(t, function (e) { e.stopPropagation(); if (t === "mousedown") e.preventDefault(); });
  });

  function start() {
    picking = true; fab.hidden = true; bar.hidden = false; form.hidden = true; box.style.display = "none"; tag.style.display = "";
    cur = null; smaller = []; down = null; dragged = false;
    document.head.appendChild(cursor);
    // Opened from the keyboard, focus was on the button that just hid itself: start Tab from the top of the page.
    if (document.activeElement === host || document.activeElement === document.body) {
      var b = document.body, had = b.hasAttribute("tabindex");
      if (!had) b.setAttribute("tabindex", "-1");
      b.focus({ preventScroll: true });
      if (!had) b.removeAttribute("tabindex");
    }
    live.textContent = "Pick mode. Click the part of the page that's wrong, or drag over an area, or Tab to it and press Enter. Arrow up picks a bigger part. Escape cancels.";
    block.forEach(function (t) { document.addEventListener(t, stop, true); });
    document.addEventListener("pointermove", move, true);
    document.addEventListener("mouseover", over, true);
    document.addEventListener("focusin", over, true);
  }
  function endPick() {
    picking = false; bar.hidden = true;
    if (cursor.parentNode) cursor.parentNode.removeChild(cursor);
    block.forEach(function (t) { document.removeEventListener(t, stop, true); });
    document.removeEventListener("pointermove", move, true);
    document.removeEventListener("mouseover", over, true);
    document.removeEventListener("focusin", over, true);
  }
  function reset() {
    endPick(); picked = null; region = null; facts = null; form.hidden = true; fab.hidden = false; box.style.display = "none"; round++; shotRound++;
    body.hidden = false; done.hidden = true; msg.hidden = true; ta.value = ""; fab.focus();
    shot = null; shotURL = ""; shotBox.hidden = true; fu.hidden = true; fuText.value = ""; acts.hidden = true; usedVoice = false;
    showInfo(false);
    main.stop(true); fuMic.stop(true);
    sync();
  }
  // A drag: the smallest element holding the whole area is what the report is about.
  function area(r) {
    var els = document.elementsFromPoint(r.left + r.width / 2, r.top + r.height / 2), el = document.body;
    for (var i = 0; i < els.length; i++) if (!ours(els[i])) { el = els[i]; break; }
    for (; el && !whole(el); el = el.parentElement) {
      var b = el.getBoundingClientRect();
      if (b.left <= r.left + 2 && b.top <= r.top + 2 && b.right >= r.right - 2 && b.bottom >= r.bottom - 2) break;
    }
    // The drag's click lands after pick mode ends; it mustn't press whatever is under the mouse.
    var eat = function (e) { e.preventDefault(); e.stopPropagation(); e.stopImmediatePropagation(); document.removeEventListener("click", eat, true); };
    document.addEventListener("click", eat, true);
    setTimeout(function () { document.removeEventListener("click", eat, true); }, 400);
    choose(el || document.body, r);
  }
  // The open menu, listbox or popover the element is in: picking from one shouldn't lose it.
  function popupOf(el) {
    return !whole(el) && el.closest && el.closest("[role=menu],[role=listbox],[role=tooltip],[data-radix-popper-content-wrapper]");
  }
  function choose(el, r) {
    endPick(); picked = el; region = r || null; facts = factsOf(el);
    if (region) frameAt(region, "Area"); else frame(el);
    var pop = popupOf(el);
    snap(false);
    tag.style.display = "none"; // the panel's chip says the same
    $(".chip code").textContent = (region ? "Area in " : "") + label(el);
    body.hidden = false; done.hidden = true; msg.hidden = true; acts.hidden = true;
    form.hidden = false; place(); showInfo(false);
    // Focus moving into the widget closes the menu (or it pulls focus back): screenshot it
    // first, then close it the way a person would, with Escape, and only then focus the box.
    if (!pop) return ta.focus();
    (shot || Promise.resolve()).then(function () {
      if (picked !== el) return;
      var t = pop.contains(document.activeElement) ? document.activeElement : pop;
      if (pop.isConnected) t.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", code: "Escape", bubbles: true, cancelable: true, composed: true }));
      setTimeout(function () { ta.focus(); }, 0);
    });
  }
  // Open the panel beside the picked element; phones get a bottom sheet (CSS).
  function place() {
    if (!picked || form.hidden) return;
    var w = form.offsetWidth, h = form.offsetHeight, W = innerWidth, H = innerHeight, g = 12, top, left;
    if (whole(picked)) {
      left = side === "left" ? 20 : W - w - 20; top = H - h - 20;
    } else {
      var r = region || picked.getBoundingClientRect();
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
  // Send works while listening: it stops the mic and waits for the last words.
  function sync() { count.textContent = ta.value.length + " / 2000"; send.disabled = !ta.value.trim() && !main.busy(); }

  // The (i) button: exactly what this report will carry, before it's sent.
  function showInfo(on) {
    infoBtn.setAttribute("aria-expanded", String(on)); info.hidden = !on;
    if (!on) return;
    var pl = payload(), dl = document.createElement("dl");
    function row(k, v) {
      var dt = document.createElement("dt"), dd = document.createElement("dd");
      dt.textContent = k; dd.textContent = v; dl.appendChild(dt); dl.appendChild(dd);
    }
    row("Page", pl.url);
    row("Element", pl.selector + (pl.text ? " · " + pl.text.slice(0, 80) : ""));
    if (pl.context) row("Where", pl.context);
    if (pl.state) row("State", pl.state);
    row("Screenshot", shotURL ? "yes, shown above" : shotBox.hidden || !offerBtn.hidden ? "no" : "being taken");
    row("Steps", pl.steps.length ? pl.steps.slice(-6).join("\n") : "none");
    row("Errors", pl.console_errors.length ? pl.console_errors.slice(-5).join("\n") : "none");
    row("Requests", pl.failed_requests.length ? pl.failed_requests.map(function (r) { return r.status + " " + r.path; }).join("\n") : "none failed");
    row("Screen", pl.viewport);
    var p = document.createElement("p");
    p.textContent = fine;
    info.innerHTML = ""; info.appendChild(dl); info.appendChild(p);
    [].forEach.call(dl.querySelectorAll("dd"), function (d) { d.style.whiteSpace = "pre-line"; });
  }

  fab.onclick = start;
  [].forEach.call(root.querySelectorAll("[data-cancel]"), function (b) { b.onclick = reset; });
  $("[data-page]").onclick = function () { choose(document.body); }; // no screenshot unless they add one
  $("[data-repick]").onclick = function () { box.style.display = "none"; start(); };
  $("[data-again]").onclick = function () { var t = type; reset(); setType(t); start(); };
  infoBtn.onclick = function () { showInfo(info.hidden); };
  offerBtn.onclick = function () { snap(true); };
  [].forEach.call(root.querySelectorAll("[data-type]"), function (b) { b.onclick = function () { setType(b.dataset.type); ta.focus(); }; });
  $(".types").onkeydown = function (e) {
    var order = ["bug", "confusing", "idea"], i = order.indexOf(type);
    if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
      e.preventDefault(); setType(order[(i + (e.key === "ArrowRight" ? 1 : 2)) % 3]); $('[data-type="' + type + '"]').focus();
    }
  };
  ta.oninput = sync;
  ta.onkeydown = function (e) { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); form.requestSubmit ? form.requestSubmit() : form.onsubmit(e); } };
  addEventListener("scroll", function () {
    if (!picked || form.hidden || body.hidden) return;
    if (region) box.style.display = "none"; // an area is where it was on screen; it doesn't follow the page
    else frame(picked);
    place();
  }, true);
  addEventListener("resize", place);

  form.onsubmit = function (e) {
    e.preventDefault();
    if (!picked || send.disabled) return;
    send.disabled = true;
    var pending = shot ? Promise.race([shot, new Promise(function (ok) { setTimeout(function () { ok(""); }, 5000); })]) : Promise.resolve("");
    Promise.all([pending, main.flush()]).then(function () {
      if (!ta.value.trim()) { sync(); return note("Say or type what's wrong first."); }
      submit();
    });
  };
  // What the element was when they picked it: menus close and lists re-render after.
  var facts = null;
  function factsOf(el) {
    if (whole(el)) return { selector: "body", tag: "body", text: "", context: "", href: "", outline: "", state: "" };
    return { selector: selector(el), tag: el.tagName.toLowerCase(), text: textOf(el), context: contextOf(el),
      href: hrefOf(el), outline: outlineOf(el), state: region ? "area picked inside it" : stateOf(el) };
  }
  function payload() {
    var f = facts || factsOf(picked);
    return {
      project: d.project, reporter: d.reporter, release: d.release || "",
      type: type, comment: ta.value, url: location.pathname,
      selector: f.selector, tag: f.tag, text: f.text, context: f.context, href: f.href, outline: f.outline, state: f.state, steps: stepsNow(),
      viewport: innerWidth + "x" + innerHeight, browser: navigator.userAgent,
      console_errors: errs.slice(), failed_requests: reqs.slice(), screenshot: shotURL
    };
  }
  function submit() {
    var mine = round;
    of0(api + "/v1/feedback", {
      method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify(payload())
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
  // Rendering copies every element under the captured one, which can freeze a page
  // showing thousands of rows; past this, the screenshot waits until they ask for it.
  // ponytail: element count is a proxy for render cost; measure real timings if it misfires.
  var bigDOM = 3000;
  function snap(force) {
    var mine = ++shotRound, el = picked, reg = region;
    shot = null; shotURL = ""; shotImg.hidden = true; shotBox.hidden = !shots;
    if (!shots) return;
    // "Just write" sends no screenshot unless they add one.
    if (whole(el) && !force) return offer("Add a screenshot of the page");
    shotWait.hidden = false; offerBtn.hidden = true; shotBar.hidden = true;
    // A frame's pause lets the panel appear before the render starts.
    shot = new Promise(function (ok) { setTimeout(ok, 32); }).then(function () { return capture(el, reg, force); })
      .then(function (url) {
        if (mine === shotRound) { shotURL = url; shotImg.src = url; shotImg.hidden = false; shotWait.hidden = true; shotBar.hidden = false; }
        return url;
      }, function (e) {
        if (mine === shotRound) { if (e === "big") offer("This part of the page is large. Add a screenshot anyway"); else shotBox.hidden = true; }
        return "";
      });
  }
  function offer(t) { shotWait.hidden = true; shotImg.hidden = true; shotBar.hidden = true; offerBtn.hidden = false; offerBtn.textContent = "+ " + t; }
  // In the copy that gets rendered: hidden parts and fields become solid blocks
  // (exactly where they are, whatever the layout), and the picked element gets a marker.
  var fieldTags = /^(INPUT|TEXTAREA|SELECT)$/;
  function blank(n) {
    if (n.nodeType !== 1) return;
    // A table keeps its rows' total height; with the rows below the area left out, the
    // copied height would stretch the rest to fill it. Tables size from their rows anyway.
    if (/^(TABLE|TBODY|THEAD|TFOOT)$/.test(n.tagName)) n.style.setProperty("height", "auto", "important");
    if (n.hasAttribute("data-lobby-picked")) {
      n.removeAttribute("data-lobby-picked");
      n.style.setProperty("outline", "2px solid rgb(255, 0, 255)", "important");
      n.style.setProperty("outline-offset", "3px", "important");
    }
    var field = fieldTags.test(n.tagName) && !/^(checkbox|radio|button|submit|reset|hidden|range|color|file|image)$/i.test(n.getAttribute("type") || "");
    var editable = n.hasAttribute("contenteditable") && n.getAttribute("contenteditable") !== "false";
    if (!field && !editable) {
      if (masks && n.hasAttribute("data-lobby-mask")) bars(n);
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
  // Draws only the area out of the rendered copy. Drawing all of root made a canvas
  // as tall as root: past the browser's canvas limit on long pages (thousands of
  // rows), so the screenshot came out blank.
  function raster(svg, area, rr, scale, bg) {
    var w = area.right - area.left, h = area.bottom - area.top, fo = svg.querySelector("foreignObject");
    fo.setAttribute("width", rr.width); fo.setAttribute("height", rr.height);
    svg.setAttribute("viewBox", (area.left - rr.left) + " " + (area.top - rr.top) + " " + w + " " + h);
    svg.setAttribute("width", w); svg.setAttribute("height", h);
    var src = "data:image/svg+xml;charset=utf-8," + encodeURIComponent(new XMLSerializer().serializeToString(svg)
      .replace(/[\u0000-\u0008\v\f\u000E-\u001F\uD800-\uDFFF\uFFFE\uFFFF]/gu, ""));
    return new Promise(function (ok, no) {
      var img = new Image();
      img.onload = function () {
        var c = document.createElement("canvas"), g = c.getContext("2d");
        c.width = Math.round(w * scale); c.height = Math.round(h * scale);
        function draw() { g.fillStyle = bg; g.fillRect(0, 0, c.width, c.height); g.drawImage(img, 0, 0, c.width, c.height); }
        // Safari and Firefox paint the images inside an SVG a little late; a second draw picks them up.
        draw(); setTimeout(function () { draw(); ok(c); }, 100);
      };
      img.onerror = function () { no(new Error("couldn't draw the screenshot")); };
      img.src = src;
    });
  }
  // reg is a dragged area: it's what gets outlined, and el is only the element holding it.
  function capture(el, reg, force) {
    var page = el === document.body || el === document.documentElement, W = innerWidth, H = innerHeight, pop = !reg && popupOf(el);
    var r = page ? { left: 0, top: 0, right: W, bottom: H, width: W, height: H } : reg || el.getBoundingClientRect(), pad = page ? 0 : reg ? 16 : 56;
    // From a dropdown: the whole dropdown, with the picked option outlined in it.
    var base = pop ? pop.getBoundingClientRect() : r;
    if (pop) pad = 4;
    var area = { left: Math.max(0, base.left - pad), top: Math.max(0, base.top - pad), right: Math.min(W, base.right + pad), bottom: Math.min(H, base.bottom + pad) };
    if (area.right - area.left < 4 || area.bottom - area.top < 4) return Promise.reject();
    // Render the smallest ancestor that holds the whole area, not the whole page.
    var root = page ? document.body : el;
    while (root !== document.body && root.parentElement) {
      var b = root.getBoundingClientRect();
      if (b.left <= area.left && b.top <= area.top && b.right >= area.right && b.bottom >= area.bottom) break;
      root = root.parentElement;
    }
    // Copying costs about a millisecond per element, so a page with thousands of rows
    // froze here. Parts entirely below the area are left out: they can't move what's
    // above them. Other parts outside it are copied as empty boxes of the same size,
    // so the layout holds.
    // ponytail: a box whose children overflow it into the area loses them; zero-size
    // wrappers (portals, anchors) are kept whole because that's where it happens most.
    // Content pinned to the bottom of a flex column could shift; revisit if seen.
    var cut = new WeakSet(), gone = new WeakSet(), kept = 0;
    var tw = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT, { acceptNode: function (n) {
      if (n === host) return NodeFilter.FILTER_REJECT;
      var b = n.getBoundingClientRect();
      if (b.width && b.height && b.top > area.bottom) { gone.add(n); return NodeFilter.FILTER_REJECT; }
      if (b.width && b.height && (b.right < area.left || b.left > area.right || b.bottom < area.top)) { cut.add(n); kept++; return NodeFilter.FILTER_REJECT; }
      kept++; return NodeFilter.FILTER_ACCEPT;
    } });
    while (tw.nextNode() && (force || kept <= bigDOM));
    if (!force && kept > bigDOM) return Promise.reject("big");
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
    // The marker goes on the picked element, or on the element holding a dragged area.
    var mark0 = !page, mr = reg ? el.getBoundingClientRect() : r;
    if (mark0) el.setAttribute("data-lobby-picked", ""); // invisible; lets the copy find it
    // Render a margin past the area: the copy can land a little off, and the marker
    // has to be in the render to tell by how much.
    var M = 96, view = { left: area.left - M, top: area.top - M, right: area.right + M, bottom: area.bottom + M };
    return loadShot().then(function (m) {
      capturing++;
      return m.domToForeignObjectSvg(root, { backgroundColor: bg, timeout: 4000, filter: function (n) { return n !== host && !gone.has(n) && !(n.parentNode && cut.has(n.parentNode)); },
        features: { restoreScrollPosition: true }, onCloneEachNode: blank })
        .then(function (svg) { return raster(svg, view, rr, scale, bg); })
        .finally(function () { capturing--; el.removeAttribute("data-lobby-picked"); });
    }, function (e) { el.removeAttribute("data-lobby-picked"); throw e; }).then(function (full) {
      rr = { left: view.left, top: view.top }; // the render starts at the view, not at root
      // The copy can lay out a few pixels off the page (margins, defaults); find the
      // picked element's marker in the render to learn the shift.
      // mk is the marker's box in the render: the picked element as the copy drew it,
      // which can be wider than on the page (no scrollbar, different clipping).
      var dx = 0, dy = 0, mk = null;
      if (mark0 && mr.top >= view.top && mr.left >= view.left) {
        var ex = Math.round((mr.left - rr.left - 5) * scale), ey = Math.round((mr.top - rr.top - 5) * scale), R = Math.round(48 * scale), X = Math.round(96 * scale);
        var x0 = Math.max(0, ex - R), y0 = Math.max(0, ey - R);
        var ww = Math.min(full.width - x0, Math.round((mr.width + 10) * scale) + R + X), hh = Math.min(full.height - y0, Math.round((mr.height + 10) * scale) + R + X);
        if (ww > 0 && hh > 0) {
          var px = full.getContext("2d").getImageData(x0, y0, ww, hh).data, mx = 1e9, my = 1e9, Mx = -1, My = -1;
          for (var i = 0; i < px.length; i += 4) if (px[i] > 250 && px[i + 1] < 6 && px[i + 2] > 249) {
            var j = i / 4, jx = j % ww, jy = Math.floor(j / ww);
            if (jx < mx) mx = jx; if (jy < my) my = jy; if (jx > Mx) Mx = jx; if (jy > My) My = jy;
          }
          if (Mx >= 0) { dx = x0 + mx - ex; dy = y0 + my - ey; mk = { x: x0 + mx, y: y0 + my, w: Mx - mx + 1, h: My - my + 1 }; }
        }
      }
      var w = Math.round((area.right - area.left) * scale), h = Math.round((area.bottom - area.top) * scale);
      var k = Math.min(1, 1400 / w), c = document.createElement("canvas");
      c.width = Math.round(w * k); c.height = Math.round(h * k);
      var g = c.getContext("2d"), s = scale * k, cx = (area.left - rr.left) * scale + dx, cy = (area.top - rr.top) * scale + dy;
      g.fillStyle = bg; g.fillRect(0, 0, c.width, c.height); // past the rendered element: page background, not black
      g.drawImage(full, cx, cy, w, h, 0, 0, c.width, c.height);
      // Where the outline goes in the output: the marker's box, or the page's measurements without one.
      // A dragged area is drawn where it was measured: the crop already moved with the shift.
      var mkBox = mk && { x: (mk.x - cx) * k, y: (mk.y - cy) * k, w: mk.w * k, h: mk.h * k }, o = reg ? 0 : 5;
      var box = mkBox && !reg ? mkBox
        : { x: (r.left - area.left - o) * s, y: (r.top - area.top - o) * s, w: (r.width + 2 * o) * s, h: (r.height + 2 * o) * s };
      // The copy can be a little wider than the page (no scrollbar, clipping), so some of
      // the marker can survive past the outline: repaint it with the page background.
      if (mark0) {
        var rb = mkBox || box, ring = 4 * s, rx = Math.max(0, Math.floor(rb.x - ring)), ry = Math.max(0, Math.floor(rb.y - ring));
        var rw = Math.min(c.width - rx, Math.ceil(rb.w + 2 * ring)), rh = Math.min(c.height - ry, Math.ceil(rb.h + 2 * ring));
        if (rw > 0 && rh > 0) {
          var probe = document.createElement("canvas").getContext("2d"), bgc;
          probe.fillStyle = bg; probe.fillRect(0, 0, 1, 1); bgc = probe.getImageData(0, 0, 1, 1).data;
          var im = g.getImageData(rx, ry, rw, rh), pd = im.data, hit = 0;
          for (var u = 0; u < pd.length; u += 4) {
            if (pd[u] - pd[u + 1] > 90 && pd[u + 2] - pd[u + 1] > 90 && Math.abs(pd[u] - pd[u + 2]) < 50) { pd[u] = bgc[0]; pd[u + 1] = bgc[1]; pd[u + 2] = bgc[2]; hit++; }
          }
          if (hit) g.putImageData(im, rx, ry);
        }
      }
      // Second layer: paint over the same parts at their measured places.
      g.fillStyle = "#9ca3af";
      masks.forEach(function (b) { g.fillRect((b.left - area.left) * s, (b.top - area.top) * s, b.width * s, b.height * s); });
      if (!page) { // outline what they picked, over the marker
        g.strokeStyle = mark; g.lineWidth = Math.max(4, 4 * s);
        g.strokeRect(box.x + s, box.y + s, box.w - 2 * s, box.h - 2 * s);
      }
      for (var q = 0.82, url = ""; q > 0.3; q -= 0.2) {
        url = c.toDataURL("image/jpeg", q);
        if (url.length < 760000) return url;
      }
      throw new Error("screenshot too large");
    });
  }

  // ---- Voice: speak instead of typing, transcribed to English ----
  // It keeps listening until they press Stop (or Send), like talking to someone:
  // each pause ends a phrase, which goes off to be transcribed while they carry on,
  // so the text fills in as they talk. Silence is never sent.
  // ponytail: the level threshold is a guess at a quiet room; tune `quiet` if
  // phrases get cut mid-sentence or noise keeps it from ever pausing.
  var quiet = 0.02, pauseMs = 1100, phraseMs = 25000, idleMs = 45000, maxMs = 300000;
  function micAllowed() {
    var fp = document.permissionsPolicy || document.featurePolicy; // a Permissions-Policy header can block the mic
    return !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia && window.MediaRecorder) &&
      (!fp || !fp.allowsFeature || fp.allowsFeature("microphone"));
  }
  // btn is the mic button; text is the box the words go into; changed runs after each phrase lands.
  function makeMic(btn, text, changed) {
    var on = null, last = Promise.resolve(), working = 0;
    function lbl(t) { btn.querySelector("span").textContent = t; }
    function go() {
      var types = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4", "audio/ogg;codecs=opus"], mt = "";
      for (var i = 0; i < types.length; i++) if (MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(types[i])) { mt = types[i]; break; }
      btn.disabled = true;
      navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } }).then(function (stream) {
        var mine = round, t0 = Date.now(), heardAt = t0, chain = Promise.resolve(), ended = false, drop = false, seg = null, floor = 0, fin;
        var ac = null, an = null, buf = null;
        try {
          ac = new (window.AudioContext || window.webkitAudioContext)();
          an = ac.createAnalyser(); an.fftSize = 1024; buf = new Uint8Array(an.fftSize);
          ac.createMediaStreamSource(stream).connect(an);
        } catch (e) { ac = null; } // no level meter: phrases are cut by length alone
        function level() {
          an.getByteTimeDomainData(buf);
          for (var i = 0, s = 0; i < buf.length; i++) { var v = (buf[i] - 128) / 128; s += v * v; }
          return Math.sqrt(s / buf.length);
        }
        function phrase() {
          var r = new MediaRecorder(stream, mt ? { mimeType: mt } : undefined), parts = [], me = { r: r, at: Date.now(), heard: !ac };
          r.ondataavailable = function (e) { if (e.data && e.data.size) parts.push(e.data); };
          r.onstop = function () {
            if (!drop && mine === round && me.heard && parts.length) transcribe(new Blob(parts, { type: r.mimeType || mt || "audio/webm" }));
            if (ended && me === seg) finish();
          };
          r.start();
          return me;
        }
        function transcribe(blob) {
          working++;
          var req = of0(api + "/v1/feedback/voice", { method: "POST", headers: { "Content-Type": blob.type, "X-Lobby-Project": d.project, "X-Lobby-Reporter": d.reporter }, body: blob })
            .then(function (res) { return res.json(); }, function () { return { error: "Couldn't reach the server. Please type instead." }; });
          // Phrases come back in any order; they go into the box in the order they were said.
          chain = chain.then(function () { return req; }).then(function (j) {
            working--;
            if (mine !== round) return;
            if (!j.ok) { note(j.error || "Couldn't transcribe that. Please type instead."); return end(false); }
            if (j.text) { text.value = (text.value.trim() ? text.value.replace(/\s+$/, "") + " " : "") + j.text; changed(); }
          }, function () { working--; });
        }
        function cut() { var old = seg; seg = phrase(); if (old.r.state !== "inactive") old.r.stop(); }
        function end(discard) {
          if (ended) return;
          ended = true; drop = discard; clearInterval(tick);
          if (seg.r.state !== "inactive") seg.r.stop(); else finish();
        }
        function finish() {
          stream.getTracks().forEach(function (t) { t.stop(); });
          if (ac) ac.close();
          on = null; btn.classList.remove("rec", "hear");
          if (working) { btn.disabled = true; lbl("Transcribing…"); live.textContent = "Transcribing what you said."; }
          chain.then(function () { btn.disabled = false; lbl("Speak"); changed(); fin(); });
        }
        seg = phrase();
        on = { end: end, done: new Promise(function (ok) { fin = ok; }) };
        last = on.done;
        btn.disabled = false; btn.classList.add("rec"); msg.hidden = true; lbl("0:00 · Stop"); changed();
        live.textContent = "Listening. Keep talking; pause between thoughts. Press Stop or Send when you're done.";
        var tick = setInterval(function () {
          var t = Date.now(), speaking = !ac;
          if (ac) {
            var lv = level();
            speaking = lv > Math.max(quiet, floor * 2.5);
            if (!speaking) floor = floor ? floor * 0.97 + lv * 0.03 : lv; // the room's own noise
          }
          if (speaking) { heardAt = t; seg.heard = true; }
          btn.classList.toggle("hear", !!ac && speaking);
          if ((ac && seg.heard && t - heardAt > pauseMs) || t - seg.at > phraseMs) cut();
          if ((ac && t - heardAt > idleMs) || t - t0 > maxMs) end(false); // nothing said for a while: stop listening
          var s = Math.floor((t - t0) / 1000);
          lbl(Math.floor(s / 60) + ":" + (s % 60 < 10 ? "0" : "") + (s % 60) + " · Stop");
        }, 100);
      }, function () {
        btn.disabled = false;
        note("The microphone is blocked. Allow it in your browser, or type instead.");
      });
    }
    btn.onclick = function () { on ? on.end(false) : go(); };
    return {
      btn: btn,
      start: function () { if (!on && talk) go(); },
      stop: function (discard) { if (on) on.end(discard); },
      busy: function () { return !!on || working > 0; },
      // Stops listening and resolves once the last phrase is in the box.
      flush: function () { if (on) on.end(false); return last; }
    };
  }
  var main = makeMic(root.querySelector(".body .mic"), ta, function () { if (main) { usedVoice = true; sync(); } });
  var fuMic = makeMic(fu.querySelector(".mic"), fuText, function () { fuSend.disabled = !fuText.value.trim() && !(fuMic && fuMic.busy()); });
  setType("bug"); sync();

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
  // The panel stays open until they close it: sending one thing often reminds
  // people of the next, and "Send another" goes straight back to picking.
  function followup(rid, mine) {
    function thanks() {
      if (mine !== round) return;
      doneState(false, "Thanks, got it", sentMsg);
      acts.hidden = false;
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
        var ch = fu.querySelector(".choices");
        ch.innerHTML = "";
        (j.choices || []).forEach(function (c) {
          var b = document.createElement("button");
          b.type = "button"; b.className = "pill"; b.textContent = c;
          b.onclick = function () { fuText.value = c; answer(); };
          ch.appendChild(b);
        });
        ch.hidden = !ch.firstChild;
        fu.hidden = false; fuText.value = ""; fuSend.disabled = true; fuText.focus();
        live.textContent = "One quick question: " + j.question;
        fu.dataset.rid = rid;
        if (usedVoice) fuMic.start(); // they talked the report through: listen for the answer too
      }, thanks);
  }
  function answer() {
    if (fuSend.dataset.busy) return;
    fuSend.disabled = true; fuSend.dataset.busy = "1";
    fuMic.flush().then(function () {
      var a = fuText.value.trim();
      if (!a) { delete fuSend.dataset.busy; return; }
      sendAnswer(a);
    });
  }
  function sendAnswer(a) {
    var mine = round;
    of0(api + "/v1/feedback/reports/" + fu.dataset.rid + "/answer", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ project: d.project, reporter: d.reporter, answer: a })
    }).then(function (r) { return r.json(); }).then(function (j) {
      if (mine !== round) return;
      fu.hidden = true; acts.hidden = false;
      doneState(false, j.ok ? "Thanks, that helps" : "Couldn't send that", j.ok ? "" : (j.error || "Please try again."));
    }, function () { fuSend.disabled = false; }).then(function () { delete fuSend.dataset.busy; });
  }
  fuText.oninput = function () { fuSend.disabled = !fuText.value.trim() && !fuMic.busy(); };
  fuText.onkeydown = function (e) { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); answer(); } };
  fuSend.onclick = answer;
  $("[data-skip]").onclick = function () { fuMic.stop(true); fu.hidden = true; doneState(false, "Thanks, got it", sentMsg); acts.hidden = false; };
  $("[data-noshot]").onclick = function () { shotRound++; shot = null; shotURL = ""; offer("Add a screenshot"); ta.focus(); };

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
  // Privacy masks are the founder's choice (off by default): with them on, nothing
  // inside data-lobby-mask is read, and screenshots show it as grey bars.
  function hidden(el) { return masks && !!el.closest("[data-lobby-mask]"); }
  // Visible text only: never input values, and nothing masked.
  var skip = /^(INPUT|TEXTAREA|SELECT|OPTION|SCRIPT|STYLE|NOSCRIPT|SVG)$/;
  function textOf(el) {
    if (hidden(el) || el.closest("[contenteditable]")) return "";
    if (/^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName)) return (el.getAttribute("aria-label") || el.getAttribute("placeholder") || el.getAttribute("name") || "").slice(0, 200);
    var out = "";
    (function walk(n) {
      for (var c = n.firstChild; c && out.length < 200; c = c.nextSibling) {
        if (c.nodeType === 3) out += " " + c.nodeValue;
        else if (c.nodeType === 1 && !skip.test(c.tagName.toUpperCase()) && !(masks && c.hasAttribute("data-lobby-mask")) && !c.isContentEditable) walk(c);
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
    if (hidden(n)) return "";
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
  // What the element looked like: disabled, loading, empty, cut off… Never values.
  function stateOf(el) {
    var out = [], cs = getComputedStyle(el), a = function (k) { return el.getAttribute(k); };
    var field = /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName);
    if (el.disabled || a("aria-disabled") === "true" || el.closest("fieldset[disabled]")) out.push("disabled");
    if (el.readOnly) out.push("read-only");
    if (el.closest("[aria-busy=true]") || el.querySelector("[aria-busy=true],[role=progressbar],[class*=spinner],[class*=skeleton],[class*=loading]") ||
      /(^|[\s_-])(loading|spinner|skeleton|pending)/i.test(typeof el.className == "string" ? el.className : "")) out.push("loading");
    if (a("aria-expanded")) out.push(a("aria-expanded") === "true" ? "expanded" : "collapsed");
    if (a("aria-selected") === "true") out.push("selected");
    if (a("aria-pressed") === "true") out.push("pressed");
    if (el.type === "checkbox" || el.type === "radio" || a("aria-checked")) out.push(el.checked || a("aria-checked") === "true" ? "checked" : "unchecked");
    if (a("aria-invalid") === "true" || (field && el.matches(":invalid") && el.value)) out.push("invalid");
    if (field) { if (!el.value) out.push("empty"); if (el.required) out.push("required"); }
    else if (!el.textContent.trim() && !el.querySelector("img,svg,canvas,video,input,iframe")) out.push("empty");
    if (cs.visibility === "hidden" || +cs.opacity < 0.1) out.push("invisible");
    if (cs.pointerEvents === "none") out.push("not clickable");
    if (!field && el.scrollWidth > el.clientWidth + 1 && /hidden|clip/.test(cs.overflowX)) out.push("text cut off");
    var r = el.getBoundingClientRect();
    if (r.width < 1 || r.height < 1) out.push("zero size");
    return out.join(", ");
  }

  // The last few things they did before reporting: clicks and page changes,
  // described like the chip (so masked text stays out). Never typed values.
  var steps = [], lastPath = location.pathname;
  function step(t) { if (t && (!steps.length || steps[steps.length - 1].t !== t)) push(steps, { t: t, at: Date.now() }, 12); }
  function moved() { if (location.pathname !== lastPath) { lastPath = location.pathname; step("go " + lastPath); } }
  function track() {
    document.addEventListener("click", function (e) {
      if (picking || !e.target || !e.target.closest || ours(e.target)) return;
      var el = e.target.closest(act) || e.target;
      if (el === document.body || el === document.documentElement) return;
      moved(); step("click " + label(el));
    }, true);
    document.addEventListener("change", function (e) {
      var el = e.target;
      if (!picking && el && /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName) && !ours(el)) step("changed " + label(el));
    }, true);
    // Client-side routers change the URL through history; note it as it happens.
    ["pushState", "replaceState"].forEach(function (k) {
      var o = history[k];
      if (typeof o == "function") history[k] = function () { var r = o.apply(this, arguments); try { moved(); } catch (e) {} return r; };
    });
    addEventListener("popstate", moved);
    addEventListener("hashchange", moved);
  }
  function stepsNow() {
    moved();
    var t = Date.now();
    return steps.map(function (x) { var s = Math.round((t - x.at) / 1000); return (s < 60 ? s + "s" : Math.round(s / 60) + "m") + " ago · " + x.t; });
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
