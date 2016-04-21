'use strict';


let initDocument = (d) => {
    Array.from(d.querySelectorAll('[data-scrollbar]')).map(attachFloatingScrollbar);
    Array.from(d.querySelectorAll('[data-tabs]')).map(attachTabBar);
    return d;
};


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


let attachFloatingScrollbar = (e) => {
    if (nativeScrollbarWidth === 0)
        return;  // native scrollbar is already floating

    let track = document.createElement('div');
    let thumb = document.createElement('div');
    thumb.classList.add('thumb');
    track.classList.add('track');
    track.classList.add('hide');
    track.appendChild(thumb);

    let timeout = null;
    let show = () => {
        thumb.style.height =  `${e.clientHeight / e.scrollHeight * track.clientHeight}px`;
        thumb.style.top    =  `${e.scrollTop    / e.scrollHeight * track.clientHeight}px`;
        track.style.top    =  `${e.scrollTop}px`;
        track.style.bottom = `-${e.scrollTop}px`;
        if (e.scrollHeight > e.clientHeight) {
            if (timeout !== null)
                window.clearTimeout(timeout);
            timeout = window.setTimeout(() => track.classList.add('hide'), 1000);
            track.classList.remove('hide');
            e.style.overflowY   = 'scroll';
            e.style.marginRight = `-${nativeScrollbarWidth}px`;
        } else {
            timeout = null;
            e.style.overflowY   = 'hidden';
            e.style.marginRight = '0';
        }
    };

    e.appendChild(track);
    e.addEventListener('scroll',    show);
    e.addEventListener('mousemove', show);
    show();
};


let attachTabBar = (e) => {
    let bar  = document.createElement('div');
    let tabs = {};
    let btns = {};
    bar.classList.add('tabbar');

    let onSelect = () => {
        let active = e.getAttribute('data-tabs');
        for (let id in tabs) {
            if (id === active) {
                btns[id].classList.add('active');
                tabs[id].removeAttribute('hidden');
            } else {
                btns[id].classList.remove('active');
                tabs[id].setAttribute('hidden', '1');
            }
        }
    };

    let setThisActive = function () {
        e.setAttribute('data-tabs', this.getAttribute('data-tab'));
    };

    for (let tab of Array.from(e.querySelectorAll('[data-tab]'))) {
        if (tab.parentElement.parentElement !== e)
            continue;
        tab.addEventListener('click', setThisActive);
        tabs[tab.getAttribute('data-tab')] = tab.parentElement;
        btns[tab.getAttribute('data-tab')] = tab;
        bar.appendChild(tab);
    }

    new MutationObserver(onSelect).observe(e, {attributes: true, attributeFilter: ['data-tabs']});
    e.insertBefore(bar, e.childNodes[0]);
    onSelect();
};


let showModal = (e) => {
    let outer  = document.createElement('div');
    let inner  = document.createElement('div');
    let scroll = document.createElement('div');
    let close  = document.createElement('a');
    outer.classList.add('modal-bg');
    inner.classList.add('modal');
    close.classList.add('button');
    close.classList.add('close');
    scroll.setAttribute('data-scrollbar', '1');
    scroll.appendChild(e);
    inner.appendChild(scroll);
    inner.appendChild(close);
    outer.appendChild(inner);

    let onFocusChange = (ev) => {
        for (let t = ev.target; t !== null; t = t.parentElement)
            if (t === outer)
                return;
        ev.target.blur();  // FIXME should somehow set focus to the dialog instead
    };

    let onClose = () => {
        outer.remove();
        document.body.removeEventListener('focusin', onFocusChange);
    };

    document.body.appendChild(initDocument(outer));
    document.body.addEventListener('focusin', onFocusChange);
    outer.addEventListener('click', (e) =>
        e.target === e.currentTarget || e.target === close ? onClose() : null);

    let randomLink = document.querySelector('a');
    if (randomLink) {
        randomLink.focus();
        randomLink.blur();
    }

    return onClose;
};


let showLoginForm = (showSignup) => {
    let template = document.querySelector('template#login-form');
    if (template === null)
        return;

    let it = initDocument(document.importNode(template.content, true)).firstElementChild;
    if (showSignup)
        it.setAttribute('data-tabs', 'signup');

    it.querySelector('.go-to-restore').addEventListener('click', (ev) => {
        ev.preventDefault();
        it.setAttribute('data-tabs', 'restore');
    });

    let closeModal = showModal(it);

    let enableForm = function () {
        it.removeAttribute('data-status');
        for (let i of Array.from(this.querySelectorAll(':read-write')))
            i.removeAttribute('disabled', '');
    };

    let disableForm = function () {
        it.setAttribute('data-status', 'loading');
        for (let i of Array.from(this.querySelectorAll(':read-write')))
            i.setAttribute('disabled', '');
    };

    let submitForm = function (ev) {
        ev.preventDefault();
        let xhr = new XMLHttpRequest();

        xhr.onload = (ev) => {
            console.log(xhr);
            if (xhr.status === 204) {
                // ...?
                window.location.reload();
            } else if (xhr.status >= 400) {
                this.classList.add('error');
                this.querySelector('.error').textContent =
                    xhr.response.getElementById('message').textContent;
            } else {
                // ...?
                window.location = xhr.responseURL;
            }
            enableForm.call(this);
        };

        xhr.onerror = (ev) => {
            this.classList.add('error');
            this.querySelector('.error').textContent = 'Could not connect to server.';
            enableForm.call(this);
        };

        xhr.responseType = 'document';
        xhr.open(this.getAttribute('method'), this.getAttribute('action'));
        xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
        xhr.send(new FormData(this));
        disableForm.call(this);
    };

    for (let form of Array.from(it.querySelectorAll('form')))
        form.addEventListener('submit', submitForm);
    return closeModal;
};


let initNavBar = (e) => {
    let onUserSwap = () => {
        let user = e.getAttribute('data-user');
        if (user !== null) {
            e.querySelector('.username').textContent = user;
        }
    };

    new MutationObserver(onUserSwap).observe(e,
        {attributes: true, attributeFilter: ['data-user']});
    onUserSwap();

    e.querySelector('.login').addEventListener('click', (ev) => {
        ev.preventDefault();
        showLoginForm(false);
    });

    e.querySelector('.signup').addEventListener('click', (ev) => {
        ev.preventDefault();
        showLoginForm(true);
    });
};


initDocument(document);
initNavBar(document.querySelector('nav'));
