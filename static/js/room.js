'use strict'; /* global $, screenfull, sha1 */

if (screenfull.enabled)
    document.addEventListener(screenfull.raw.fullscreenchange, _ => {
        // browser support for :fullscreen is abysmal.
        for (let e of document.querySelectorAll('.is-fullscreen'))
            e.classList.remove('is-fullscreen');
        if (screenfull.element)
            screenfull.element.classList.add('is-fullscreen');
    });
else
    document.body.classList.add('no-fullscreen');


Element.prototype.button = function (selector, f) {
    for (let e of this.querySelectorAll(selector)) {
        e.addEventListener('click', ev => ev.preventDefault());
        e.addEventListener('click', f);
    }
};


document.body.addEventListener('keydown', ev => {
    if (ev.keyCode === 83 && ev.ctrlKey && ev.shiftKey)
        document.body.classList.toggle('aside-chat');
    if (ev.keyCode === 65 && ev.ctrlKey && ev.shiftKey)
        document.querySelector('.player').classList.toggle('has-video');
});


let RPC = function () {
    this.nextID   = 0;
    this.awaiting = new Object(null);
    this.handlers = new Object(null);
    this.on('RPC.Redirect', url =>
        this.open(url.substr(0, 2) == "//" ? this.url.substr(0, this.url.indexOf(":") + 1) + url : url));
    this.on('RPC.Loaded', _ =>
        this.emit(this.state = RPC.STATE_OPEN));
};


RPC.STATE_INIT   = Symbol();
RPC.STATE_OPEN   = Symbol();
RPC.STATE_CLOSED = Symbol();


RPC.prototype.open = function (url) {
    if (this.socket)
        this.socket.close();
    this.socket = new WebSocket(this.url = url);
    this.socket.onmessage = ev => {
        let msg = JSON.parse(ev.data);
        if (msg.method)
            this.emit(msg.method, ...msg.params);
        if (msg.id in this.awaiting) {
            let cb = this.awaiting[msg.id];
            delete this.awaiting[msg.id];
            if (msg.error)
                cb.reject(msg.error);
            else
                cb.resolve(msg.result);
        }
    };
    this.socket.onclose = _ => this.emit(this.state = RPC.STATE_CLOSED);
    this.emit(this.state = RPC.STATE_INIT);
};


RPC.prototype.on = function (ev, cb) {
    if (ev === this.state) cb();
    this.handlers[ev] = this.handlers[ev] || [];
    this.handlers[ev].push(cb);
};


RPC.prototype.emit = function (ev, ...params) {
    for (let f of (this.handlers[ev] || []))
        f(...params);
};


RPC.prototype.send = function (method, ...params) {
    let id = this.nextID++;
    this.socket.send(JSON.stringify({ jsonrpc: '2.0', id, method, params }));
    return new Promise((resolve, reject) => { this.awaiting[id] = { resolve, reject }; });
};


$.form.onDocumentReload = doc => {
    let move = (src, selector, dst) => {
        src = src.querySelectorAll(selector);
        dst = dst.querySelectorAll(selector);
        for (let i = 0; i < src.length && i < dst.length; i++)
            dst[i].parentElement.replaceChild(src[i], dst[i]);
        return src;
    };

    move(document, '.stream-header .viewers', doc);
    for (let e of move(doc, 'nav, .stream-header, .stream-meta, footer', document))
        $.init(e);
    for (let modal of document.querySelectorAll('x-modal-cover'))
        modal.remove();
    return true;
};


let withRPC = rpc => ({
    '.viewers'(e) {
        rpc.on('Stream.ViewerCount', n => e.textContent = n);
    },

    '.player'(e) {
        rpc.on(RPC.STATE_INIT, _ => e.dataset.status = 'loading');
        rpc.on(RPC.STATE_OPEN, _ => {
            // TODO measure connection speed, request a stream
            e.dataset.src = rpc.url.replace('ws', 'http');
            e.dataset.live = '1';
        });
        rpc.on(RPC.STATE_CLOSED, _ => {
            if (e.dataset.live) delete e.dataset.live;
            e.dataset.src = '';
        });
    },

    '.chat'(e) {
        let log = e.querySelector('.log');
        let autoscroll = f => (...args) => {
            let atBottom = log.scrollTop + log.clientHeight >= log.scrollHeight;
            f(...args);
            if (atBottom) log.scrollTop = log.scrollHeight;
        };

        let submitRPCRequest = ev => {
            let f = ev.target;
            let i = f.querySelector('[data-arg]');
            ev.preventDefault();
            $.form.disable(f);
            rpc.send(f.dataset.rpc, i.value).then(autoscroll(_ => {
                $.form.enable(f);
                f.classList.remove('error');
                i.value = '';
                i.select();
            })).catch(autoscroll(err => {
                $.form.enable(f);
                f.classList.add('error');
                f.querySelector('.error').textContent = err.message;
            }));
        };

        for (let f of e.querySelectorAll('form[data-rpc]'))
            f.addEventListener('submit', submitRPCRequest);

        e.button('.ins-emoji', ev => {
            let f = e.querySelector('.input-form');
            let i = f.querySelector('textarea');
            let s = $.emoji();
            f.appendChild(s);
            s.addEventListener('select', ev => {
                s.remove();
                i.value += ev.detail;
                i.focus();
            });
        });

        rpc.on(RPC.STATE_OPEN,   autoscroll(_ => e.classList.add('online')));
        rpc.on(RPC.STATE_CLOSED, autoscroll(_ => e.classList.remove('online')));
        rpc.on('Chat.Message', autoscroll((name, text, login) => {
            let h = parseInt(sha1(`${login}\n${name}`).slice(32), 16);
            let m = document.createElement('li');
            let nameSpan = document.createElement('span');
            let textSpan = document.createElement('span');
            nameSpan.classList.add('name');
            nameSpan.style.color = `hsl(${h % 359},${(h / 359|0) % 60 + 30}%,${((h / 359|0) / 60|0) % 30 + 50}%)`;
            nameSpan.textContent = name;
            nameSpan.setAttribute('title', login);
            textSpan.textContent = text;
            textSpan.innerHTML = $.emoji.parse(textSpan.innerHTML);
            m.appendChild(nameSpan);
            m.appendChild(textSpan);
            log.appendChild(m);
        }));
        rpc.on('Chat.AcquiredName', autoscroll((name, login) => {
            e.classList.add('logged-in');
            e.querySelector('.input-form textarea').select();
        }));
    },
});


let confirmMaturity = e => new Promise(resolve => {
    if (!e.hasAttribute('data-unconfirmed'))
        return resolve();
    let confirm = _ => {
        localStorage.mature = '1';
        for (let c of e.querySelectorAll('.nsfw-message'))
            c.remove();
        delete e.dataset.unconfirmed;
        resolve();
    };
    if (!!localStorage.mature)
        confirm();
    e.button('.confirm-age', confirm);
});


$.extend({
    '[data-stream-id]'(e) {
        let rpc = new RPC();
        $.apply(e, withRPC(rpc));
        confirmMaturity(e).then(() =>
            rpc.open(`${location.protocol.replace('http', 'ws')}//${location.host}/stream/${encodeURIComponent(e.dataset.streamId)}`));
    },

    '[data-stream-src]'(e) {
        confirmMaturity(e).then(() => e.querySelector('.player').dataset.src = e.dataset.streamSrc);
    },

    '.player-block'(e) {
        e.button('.theatre',  _ => e.classList.add('theatre'));
        e.button('.collapse', _ => e.classList.remove('theatre'));
    },

    '.player'(e) {
        // TODO playing, waiting, stalled (not sure whether these events are actually emitted)
        let video  = e.querySelector('video');
        let status = e.querySelector('.status');
        let volume = e.querySelector('.volume');
        let seek   = e.querySelector('.seek');

        let setStatus = (short, long) => {
            e.dataset.status = short;
            status.textContent = long || short;
        };

        let setError = code => setStatus(
              code === 4 ? (e.dataset.live ? 'stopped' : 'ended') : 'error',
              code === 4 ? (e.dataset.live ? 'stopped' : 'stream ended')
            : code === 3 ? 'decoding error'
            : code === 2 ? 'network error'
            : /* code === 1 ? */ 'aborted');

        let setTime = t => {
            if (video.error)
                return setError(video.error.code);
            // let leftPad = require('left-pad');
            setStatus(video.paused ? 'paused' : 'playing', `${(t / 60)|0}:${t % 60 < 10 ? '0' : ''}${(t|0) % 60}`);
            seek.dataset.value = t / (video.duration || t || 1);
        };

        let setVolume = (vol, muted) => {
            localStorage.volume = volume.dataset.value = vol;
            localStorage.muted  = muted ? '1' : '';
            e.classList[muted ? 'add' : 'remove']('muted');
        };

        let ignoreErrors = p => { if (p) p.catch(e => null); };
        let seekTo = $.delayedPair(50,
            x => { video.pause(); setTime(x); },
            x => { video.currentTime = x; ignoreErrors(video.play()) });

        let vol = +localStorage.volume;
        setVolume(video.volume = isNaN(vol) ? 1 : Math.min(1, Math.max(0, vol)),
                  video.muted  = !!localStorage.muted);

        seek.addEventListener('change', ev => seekTo(ev.detail * video.duration));
        volume.addEventListener('change', ev => video.muted = !(video.volume = ev.detail));
        video.addEventListener('loadstart',      _ => setStatus('loading'));
        video.addEventListener('loadedmetadata', _ => setStatus('loading', 'buffering'));
        video.addEventListener('ended',          _ => setError(4 /* "unsupported media" */));
        video.addEventListener('error',          _ => setError(video.error.code));
        video.addEventListener('playing',        _ => setTime(video.currentTime));
        video.addEventListener('timeupdate',     _ => setTime(video.currentTime));
        video.addEventListener('volumechange',   _ => setVolume(video.volume, video.muted));
        $.observeData(e, 'src', '', src => (video.src = src) ? ignoreErrors(video.play()) : setError(4));

        e.button('.play', _ => {
            if (e.dataset.live)
                e.dataset.src = e.dataset.src;
            else
                ignoreErrors(video.play());
        });

        e.button('.stop', _ => {
            setStatus(e.dataset.live ? 'stopped' : 'paused', status.textContent);
            if (e.dataset.live)
                video.src = '';
            else
                video.pause();
        });

        e.button('.mute',       _ => video.muted = true);
        e.button('.unmute',     _ => video.muted = false);
        e.button('.fullscreen', _ => screenfull.request(e));
        e.button('.collapse',   _ => screenfull.exit());

        let showControls = $.delayedPair(3000,
            () => e.classList.remove('hide-controls'),
            () => e.classList.add('hide-controls'));

        e.addEventListener('mousemove', showControls);
        e.addEventListener('focusin',   showControls);
        e.addEventListener('keydown',   showControls);
    },

    '.stream-header'(e) {
        e.button('.edit', ev => {
            let f = $.template('edit-name-template').querySelector('form');
            let i = f.querySelector('input');
            f.addEventListener('reset',  _  => f.remove());
            ev.currentTarget.parentElement.insertBefore(f, ev.currentTarget);
            i.value = e.querySelector('.name').textContent;
            i.focus();
        });
    },

    '.stream-about x-panel'(e) {
        e.button('.edit', ev => {
            let f = $.template('edit-panel-template').querySelector('form');
            let i = f.querySelector('textarea');
            f.addEventListener('reset', _ => f.remove());
            if ((f.querySelector('[name="id"]').value = ev.currentTarget.dataset.panel))
                f.querySelector('.remove').addEventListener('click', () =>
                    f.setAttribute('action', '/user/del-stream-panel'));
            else
                f.querySelector('.remove').remove();
            e.insertBefore(f, e.children[0]);
            i.value = e.querySelector('[data-markup=""]').textContent;
            i.focus();
        });
    },
});
