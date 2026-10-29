// Shared by every page: the nav's border on scroll, the GitHub star count,
// and, on docs pages, copy buttons on code and the "On this page" highlight.
(() => {
  const nav = document.getElementById('nav');
  const onScroll = () => nav.classList.toggle('scrolled', scrollY > 8);
  addEventListener('scroll', onScroll, { passive: true }); onScroll();

  // Close the mobile menu after following a link to an anchor on the same page.
  document.querySelectorAll('.menu a, .docs-picker a').forEach(a => a.addEventListener('click', () => a.closest('details').open = false));

  const icon = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/></svg>';
  document.querySelectorAll('.prose pre').forEach(pre => {
    const b = document.createElement('button');
    b.className = 'copy'; b.type = 'button'; b.setAttribute('aria-label', 'Copy'); b.innerHTML = icon;
    b.addEventListener('click', () => {
      const text = pre.querySelector('code').innerText.replace(/\n$/, '');
      navigator.clipboard && navigator.clipboard.writeText(text).then(() => {
        b.classList.add('ok'); setTimeout(() => b.classList.remove('ok'), 1400);
      }, () => {});
    });
    pre.appendChild(b);
  });

  const links = [...document.querySelectorAll('.toc a')];
  if (links.length && 'IntersectionObserver' in window) {
    const heads = links.map(a => document.getElementById(a.hash.slice(1)));
    const mark = () => {
      let i = 0;
      heads.forEach((h, j) => { if (h.getBoundingClientRect().top < 140) i = j; });
      links.forEach((a, j) => a.classList.toggle('on', j === i));
    };
    addEventListener('scroll', mark, { passive: true }); mark();
  }

  const stars = document.querySelector('[data-stars]');
  if (stars) fetch('https://api.github.com/repos/rohansx/lobby').then(r => r.ok ? r.json() : null).then(d => {
    if (!d || d.stargazers_count < 10) return;
    stars.textContent = d.stargazers_count >= 1000 ? (d.stargazers_count / 1000).toFixed(1) + 'k' : d.stargazers_count;
    stars.hidden = false;
  }).catch(() => {});
})();
