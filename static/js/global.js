'use strict';


let nativeScrollbarWidth = (() => {
    let e = document.createElement('div');
    e.style.position = 'absolute';
    e.style.top      = '-200px';
    e.style.width    = '100px';
    e.style.height   = '100px';
    e.style.overflow = 'scroll';
    document.body.appendChild(e);
    let r = e.offsetWidth - e.clientWidth;
    e.remove();
    return r;
})();


let createCustomScrollbar = (e) => {
    if (nativeScrollbarWidth === 0)
        return;

    e.style.overflowY   = 'scroll';
    e.style.marginRight = `-${nativeScrollbarWidth}px`;

    let track = document.createElement('div');
    let thumb = document.createElement('div');
    thumb.classList.add('thumb');
    track.classList.add('scrollbar');
    track.classList.add('hidden');
    track.appendChild(thumb);

    let timeout = null;
    let show = () => {
        thumb.style.top          =  `${e.scrollTop    / e.scrollHeight * track.clientHeight}px`;
        thumb.style.height       =  `${e.clientHeight / e.scrollHeight * track.clientHeight}px`;
        track.style.marginTop    =  `${e.scrollTop}px`;
        track.style.marginBottom = `-${e.scrollTop}px`;
        if (e.scrollHeight > e.clientHeight) {
            if (timeout !== null)
                window.clearTimeout(timeout);
            track.classList.remove('hidden');
            timeout = window.setTimeout(() => track.classList.add('hidden'), 1000);
        } else
            timeout = null;
    };

    e.appendChild(track);
    e.addEventListener('scroll',    show);
    e.addEventListener('mousemove', show);
};


for (let es = document.querySelectorAll('[data-scrollable]'), i = 0; i < es.length; i++)
    createCustomScrollbar(es[i]);
