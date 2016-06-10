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


const RPC_STATE_NULL     = 0;
const RPC_STATE_INIT     = 1;
const RPC_STATE_OPEN     = 2;
const RPC_STATE_CLOSED   = 3;


let RPC = function () {
    this.nextID   = 0;
    this.state    = RPC_STATE_NULL;
    this.objects  = [];
    this.awaiting = {};
    this.handlers = {
        'RPC.Redirect': url => {
            if (url.substr(0, 2) == "//")
                url = (this.url.substr(0, 4) == "wss:" ? "wss:" : "ws:") + url;
            this.open(url);
        },

        'RPC.Loaded': _ => {
            this.state = RPC_STATE_OPEN;
            for (let object of this.objects)
                object.load();
        }
    };
};


RPC.prototype.open = function (url) {
    if (this.socket)
        this.socket.close();

    this.state  = RPC_STATE_INIT;
    this.socket = new WebSocket(this.url = url);
    this.socket.onclose = _ => {
        this.state = RPC_STATE_CLOSED;
        for (let object of this.objects)
            object.unload();
    };

    this.socket.onmessage = ev => {
        let msg = JSON.parse(ev.data);

        if (msg.method in this.handlers)
            this.handlers[msg.method](...msg.params);

        if (msg.id in this.awaiting) {
            let cb = this.awaiting[msg.id];
            delete this.awaiting[msg.id];
            if (msg.error)
                cb.reject(msg.error);
            else
                cb.resolve(msg.result);
        }
    };

    for (let object of this.objects)
        if (object.open)
            object.open();
};


RPC.prototype.register = function (obj) {
    if (this.state >= RPC_STATE_INIT && obj.open)
        obj.open();
    if (this.state >= RPC_STATE_OPEN)
        obj.load();
    if (this.state >= RPC_STATE_CLOSED)
        obj.unload();
    this.objects.push(obj);
};


RPC.prototype.send = function (method, ...params) {
    let id = this.nextID++;
    this.socket.send(JSON.stringify({ jsonrpc: '2.0', id, method, params }));
    return new Promise((resolve, reject) => { this.awaiting[id] = { resolve, reject }; });
};


$.form.onDocumentReload = doc => {
    let move = (src, dst, selector) => {
        let a = src.querySelector(selector);
        let b = dst.querySelector(selector);
        if (a && b) {
            b.parentElement.replaceChild(a, b);
            b.remove();
            if (dst === document)
                $.init(a);
        }
    };

    move(document, doc, '.stream-header .viewers');
    move(doc, document, '.stream-header');
    move(doc, document, '.stream-meta');
    move(doc, document, 'nav');
    for (let modal of document.querySelectorAll('x-modal-cover'))
        modal.remove();
    return true;
};


let withRPC = rpc => ({
    '.viewers'(e) {
        rpc.handlers['Stream.ViewerCount'] = n => e.textContent = n;
    },

    '.player'(e) {
        rpc.register({
            open: () =>
                e.dataset.status = 'loading',
            load: () => {
                // TODO measure connection speed, request a stream
                e.dataset.src = rpc.url.replace('ws', 'http');
                e.dataset.live = '1';
            },
            unload: () => {
                delete e.dataset.live;
                e.dataset.src = '';
            },
        });
    },

    '.chat'(root) {
        let log  = root.querySelector('.log');
        let form = root.querySelector('.input-form');
        let text = root.querySelector('.input-form .input');

        let autoscroll = m => (...args) => {
            let scroll = log.scrollTop + log.clientHeight >= log.scrollHeight;
            m(...args);
            if (scroll)
                log.scrollTop = log.scrollHeight;
        };

        let handleErrors = (form, promise, withMessage) => {
            $.form.disable(form);
            return promise.then(autoscroll(() => {
                $.form.enable(form);
                form.classList.remove('error');
            })).catch(autoscroll((e) => {
                $.form.enable(form);
                form.classList.add('error');
                form.querySelector('.error').textContent = e.message;
            }));
        };

        root.querySelector('.login-form').addEventListener('submit', function (ev) {
            ev.preventDefault();
            handleErrors(this, rpc.send('Chat.SetName', this.querySelector('.input').value));
        });

        form.addEventListener('submit', ev => {
            ev.preventDefault();
            handleErrors(form, rpc.send('Chat.SendMessage', text.value).then(() => {
                log.scrollTop = log.scrollHeight;
                text.value = '';
                text.select();
            }), true);
        });

        let stringColor = str => {
            let h = parseInt(sha1(str).slice(32), 16);
            return `hsl(${h % 359},${(h / 359|0) % 60 + 30}%,${((h / 359|0) / 60|0) % 30 + 50}%)`;
        };

        rpc.handlers['Chat.Message'] = autoscroll((name, text, login) => {
            let entry = $.template('chat-message-template');
            let e = entry.querySelector('.name');
            // TODO maybe do this server-side? that'd allow us to hash the IP instead...
            e.style.color = stringColor(`${name.length}:${name}${login}`);
            e.textContent = name;
            e.setAttribute('title', login || 'Anonymous user');
            if (!login)
                e.classList.add('anon');
            entry.querySelector('.text').textContent = text;
            log.appendChild(entry);
        });

        rpc.handlers['Chat.AcquiredName'] = autoscroll((name, login) => {
            if (name === "") {
                root.classList.remove('logged-in');
                root.querySelector('.login-form').classList.add('error');
            } else {
                root.classList.add('logged-in');
                text.select();
            }
        });

        rpc.register({
            load() {
                rpc.send('Chat.RequestHistory');
                root.classList.add('online');
            },

            unload() {
                root.classList.remove('online');
            },
        });
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
    else
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
            // let leftPad = require('left-pad');
            setStatus(video.paused ? 'paused' : 'playing', `${(t / 60)|0}:${t % 60 < 10 ? '0' : ''}${(t|0) % 60}`);
            seek.dataset.value = t / (video.duration || t || 1);
        };

        let setVolume = (vol, muted) => {
            localStorage.volume = volume.dataset.value = vol;
            localStorage.muted  = muted ? '1' : '';
            e.classList[muted ? 'add' : 'remove']('muted');
        };

        let seekTo = $.delayedPair(50,
            x => { video.pause(); setTime(x); },
            x => { video.currentTime = x; video.play().catch(e => null); });

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
        $.observeData(e, 'src', '', src => (video.src = src) ? video.play() : setError(4));

        e.button('.play', _ => {
            if (e.dataset.live)
                e.dataset.src = e.dataset.src;
            else
                video.play();
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

    '.stream-about'(e) {
        e.button('.edit', ev => {
            let f = $.template('edit-panel-template').querySelector('form');
            let i = f.querySelector('textarea');
            f.addEventListener('reset', _ => f.remove());
            if ((f.querySelector('[name="id"]').value = ev.currentTarget.dataset.panel))
                f.querySelector('.remove').addEventListener('click', () =>
                    f.setAttribute('action', '/user/del-stream-panel'));
            else
                f.querySelector('.remove').remove();
            ev.currentTarget.parentElement.insertBefore(f, ev.currentTarget);
            i.value = ev.currentTarget.parentElement.querySelector('[data-markup=""]').textContent;
            i.focus();
        });
    },
});
