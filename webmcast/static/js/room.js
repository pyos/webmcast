'use strict';


let RPC = {
    CODE_SET_NAME:     0,
    CODE_SEND_MESSAGE: 1,
    CODE_GET_HISTORY:  2,
    CODE_MEASURE_RATE: 3,

    WebSocket: function (socket) {
        let cbs_by_id   = {};
        let cbs_by_code = {};
        let id = 0;

        socket.binaryType = 'arraybuffer';
        socket.onmessage = (ev) => {
            let msg = RPC.decode(ev.data);

            if (msg.id == 0xFFFF) {
                if (msg.code in cbs_by_code)
                    cbs_by_code[msg.code](...msg.args);
            }

            if (msg.id in cbs_by_id) {
                let cb = cbs_by_id[msg.id];
                delete cbs_by_id[msg.id];
                if (msg.code)
                    cb.resolve(...msg.args);
                else {
                    console.log(msg.args);
                    cb.reject(...msg.args);
                }
            }
        };

        let send = (code, ...objects) =>
            new Promise((resolve, reject) => {
                socket.send(RPC.encode(id, code, ...objects));
                cbs_by_id[id] = { resolve, reject };
                id = (id + 1) & 0x7FFF;
            });

        let callback = (code, cb) => {
            if (cb === undefined)
                delete cbs_by_code[code];
            else
                cbs_by_code[code] = cb;
        };

        return { send, callback };
    },

    decode: (buffer) => {
        let view = new DataView(buffer);
        let id   = view.getUint16(0);
        let code = view.getUint16(2);
        let args = [];

        for (let i = 4; i < buffer.byteLength;) {
            let r = RPC.decode1(buffer, view, i);
            args.push(r.value);
            i = r.i;
        }

        return { id, code, args };
    },

    decode1: (buffer, view, i) => {
        switch (view.getUint8(i++)) {
            case 0: return { i, value: false };
            case 1: return { i, value: true  };
            case 2: return { i, value: null  };
            case 3: return { i: i + 4, value: view.getInt32(i) };
            case 4: return { i: i + 8, value: view.getFloat64(i) };
            case 5: {
                let length = view.getUint32(i);
                let part = buffer.slice(i + 4, i + 4 + length);
                return { i: i + 4 + length, value: new TextDecoder('utf-8').decode(part) };
            }
            case 6: {
                let length = view.getUint32(i);
                return { i: i + 4 + length, value: buffer.slice(i + 4, i + 4 + length) };
            }
            case 7: {
                let value = new Array(view.getUint16(i));
                i += 2;
                for (let k = 0; k < value.length; k++) {
                    let r = RPC.decode1(buffer, view, i);
                    value[k] = r.value;
                    i = r.i;
                }
                return { i, value };
            }
            case 8: {
                let value = {};
                let size = view.getUint16(i);
                i += 2;
                while (size--) {
                    let k = RPC.decode1(buffer, view, i);
                    let v = RPC.decode1(buffer, view, k.i);
                    value[k.value] = v.value;
                    i = v.i;
                }
                return { i, value };
            }
        }

        throw `unknown type: ${view.getUint8(i - 1)}`;
    },

    encode: (id, code, ...args) => {
        let data = new Uint8Array(256);
        let view = new DataView(data.buffer);
        view.setUint16(0, id);
        view.setUint16(2, code);

        let r = { i: 4, data, view };
        for (let arg of args)
            r = RPC.encode1(r, arg);
        return r.data.buffer.slice(0, r.i);
    },

    ensure: (r, n) => {
        if (r.data.length < r.i + n) {
            let old = r.data;
            r.data = new Uint8Array(r.i + n + 256);
            r.view = new DataView(r.data.buffer);
            r.data.set(old, 0);
        }
        return r;
    },

    encode1: (r, x) => {
        r = RPC.ensure(r, 16);
        if      (x === false) r.view.setUint8(r.i++, 0);
        else if (x === true)  r.view.setUint8(r.i++, 1);
        else if (x === null)  r.view.setUint8(r.i++, 2);
        else if (x === +x && x === (x|0)) {
            r.view.setUint8(r.i++, 3);
            r.view.setInt32(r.i, x);
            r.i += 4;
        }
        else if (x === +x && x !== (x|0)) {
            r.view.setUint8(r.i++, 4);
            r.view.setFloat64(r.i, x);
            r.i += 8;
        }
        else if (typeof x === "string") {
            let data = new TextEncoder('utf-8').encode(x);
            r = RPC.ensure(r, 5 + data.length);
            r.view.setUint8(r.i++, 5);
            r.view.setUint32(r.i, data.length);
            r.data.set(data, r.i + 4);
            r.i += 4 + data.length;
        }
        else if (x instanceof ArrayBuffer) {
            r = RPC.ensure(r, 5 + x.byteLength);
            r.view.setUint8(r.i++, 6);
            r.view.setUint32(r.i, x.byteLength);
            r.data.set(new Uint8Array(x), r.i + 4);
            r.i += 4 + x.byteLength;
        }
        else if (x instanceof Array) {
            r.view.setUint8(r.i++, 7);
            r.view.setUint16(r.i, x.length);
            r.i += 2;
            for (let y of x)
                r = RPC.encode1(r, y);
        }
        else {
            let n = 0; for (let k in x) n++;
            r.view.setUint8(r.i++, 8);
            r.view.setUint16(r.i, n);
            r.i += 2;
            for (let k in x)
                r = RPC.encode1(RPC.encode1(r, k), x[k]);
        }
        return r;
    },
};


let ViewNode = function (root, stream) {
    let view = root.querySelector('video');
    let rpc  = null;

    view.addEventListener('loadstart', () => {
        root.classList.remove('uk-icon-warning');
        root.classList.add('w-icon-loading');
    });

    view.addEventListener('loadedmetadata', () => {
        root.classList.remove('uk-icon-warning');
        root.classList.remove('w-icon-loading');
        root.querySelector('.pad').remove();
    });

    view.addEventListener('error', () => {
        root.classList.remove('w-icon-loading');
        root.classList.add('uk-icon-warning');
    });

    view.addEventListener('ended', () => {
        root.classList.remove('w-icon-loading');
        root.classList.add('uk-icon-warning');
    });

    let onLoad = (socket) => {
        rpc = socket;
        // TODO measure connection speed, request a stream
        view.src = `/stream/${stream}`;
        view.play();
    };

    let onUnload = () => {
        rpc = null;
    };

    let measure = (size) => {
        if (!rpc)
            return new Promise((resolve, _) => resolve(Infinity));

        const start = window.performance.now();
        console.log(start);

        return rpc.send(RPC.CODE_MEASURE_RATE, size).then(() => {
            const end = window.performance.now();
            console.log(end);
            return (end - start) / 1000;
        });
    };

    return { onLoad, onUnload, measure };
};


let ChatNode = function (root) {
    let log = root.querySelector('.log');
    let msg = log.querySelector('.message');
    let rpc = null;
    msg.remove();

    let onLoad = (socket) => {
        rpc = socket;
        rpc.callback(RPC.CODE_SEND_MESSAGE, (name, text) => {
            let entry = msg.cloneNode(true);
            entry.querySelector('.name').textContent = name;
            entry.querySelector('.text').textContent = text;
            log.appendChild(entry);
        });

        rpc.send(RPC.CODE_GET_HISTORY);
        root.classList.add('active');
    };

    let onUnload = () => {
        rpc = null;
        root.classList.remove('active');
    };

    let form = root.querySelector('.input-form');
    let text = form.querySelector('.input');

    text.addEventListener('keydown', (ev) =>
        (ev.keyCode === 13 && !ev.shiftKey ? ev.preventDefault() : null));

    text.addEventListener('keyup', (ev) =>
        (ev.keyCode === 13 && !ev.shiftKey ? form.dispatchEvent(new Event('submit')) : null));

    form.addEventListener('submit', (ev) => {
        ev.preventDefault();
        if (rpc && text.value) {
            rpc.send(RPC.CODE_SEND_MESSAGE, text.value).then(() => {
                text.value = '';
                text.focus();
            });
        }
    });

    let lform = root.querySelector('.login-form');
    let login = lform.querySelector('.input');

    lform.addEventListener('submit', (ev) => {
        ev.preventDefault();
        if (rpc && login.value) {
            rpc.send(RPC.CODE_SET_NAME, login.value).then(() => {
                lform.remove();
                text.focus();
            });
        }
    });

    return { onLoad, onUnload };
};


let stream = document.body.getAttribute('data-stream-id');
let view = new ViewNode(document.querySelector('.w-view-container'), stream);
let chat = new ChatNode(document.querySelector('.w-chat-container'));
let socket = new WebSocket(`ws${window.location.protocol == 'https:' ? 's' : ''}://`
                           + `${window.location.host}/stream/${stream}`);
let rpc = new RPC.WebSocket(socket);

socket.onopen = () => {
    chat.onLoad(rpc);
    view.onLoad(rpc);
};

socket.onclose = (ev) => {
    chat.onUnload();
    view.onUnload();
};

// TODO socket.onerror?
