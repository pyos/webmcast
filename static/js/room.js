'use strict';


let RPC = {
    WebSocket: function (socket) {
        let cbs_by_id   = {};
        let cbs_by_code = {};
        let id = 0;

        socket.onmessage = (ev) => {
            let msg = JSON.parse(ev.data);

            if (msg.id === undefined) {
                if (msg.method in cbs_by_code)
                    cbs_by_code[msg.method](...msg.params);
            }

            if (msg.id in cbs_by_id) {
                let cb = cbs_by_id[msg.id];
                delete cbs_by_id[msg.id];
                if (msg.error === undefined)
                    cb.resolve(msg.result);
                else
                    cb.reject(msg.error.message, msg.error.code, msg.error.data);
            }
        };

        let send = (method, ...params) =>
            new Promise((resolve, reject) => {
                socket.send(JSON.stringify({ jsonrpc: '2.0', id, method, params }));
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

        return rpc.send('.get_zeros', size).then(() => {
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
        rpc.callback('chat_message', (name, text) => {
            let entry = msg.cloneNode(true);
            entry.querySelector('.name').textContent = name;
            entry.querySelector('.text').textContent = text;
            log.appendChild(entry);
        });

        rpc.send('Chat.RequestHistory');
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
            rpc.send('Chat.SendMessage', text.value).then(() => {
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
            rpc.send('Chat.SetName', login.value).then(() => {
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
