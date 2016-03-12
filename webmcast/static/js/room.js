'use strict';


let wcast_nop_struct = {
    on_open:    (ws) => null,
    on_close:   ()   => null,
    on_error:   ()   => null,
    on_message: ()   => null,
};


let wcast_view_init = (root, stream) => {
    if (root === null)
        return wcast_nop_struct;

    let self = {};
    let view = root.querySelector('.view');

    self.on_open = (ws) => {
        // TODO measure connection speed, request a stream
        view.src = `/stream/${stream}`;
        view.play();
    };

    self.on_close = () => {
        // TODO destroy the view
    };

    self.on_error = () => {
        // TODO something
    };

    return self;
};


let wcast_chat_init = (root) => {
    if (root === null)
        return wcast_nop_struct;

    let input        = root.querySelector('.input');
    let input_form   = root.querySelector('.input-form');
    let message_list = root.querySelector('.log');
    let message_tpl  = root.querySelector('.message');

    input.setAttribute('disabled', '');
    message_list.removeChild(message_tpl);

    let self = {};
    let socket = null;

    self.insert_entry = (name, text, add_class) => {
        let elem = message_tpl.cloneNode(true);
        elem.querySelector('.name').textContent = name;
        elem.querySelector('.text').textContent = text;
        if (add_class)
            elem.classList.add(add_class);
        message_list.appendChild(elem);
    };

    self.on_open = (ws) => {
        socket = ws;
        input.removeAttribute('disabled');
    };

    self.on_close = () => {
        socket = null;
        input.setAttribute('disabled', '');
        self.insert_entry('', 'disconnected', 'status');
    };

    self.on_error = () => {
        self.insert_entry('', 'connection error', 'status');
    };

    self.on_message = (m) => {
        self.insert_entry('random', m);
    };

    input_form.addEventListener('submit', (ev) => {
        ev.preventDefault();

        if (input.value && socket) {
            socket.send(input.value);
            input.value = '';
            input.focus();
        }
    });

    return self;
};


let wcast_stream_view_init = (root, wshost) => {
    let stream = root.getAttribute('data-stream-id');
    if (stream === null)
        return null;

    let self = {};

    if ((self.view = wcast_view_init(root.querySelector('.w-view-container'), stream)) === null)
        return null;

    if ((self.chat = wcast_chat_init(root.querySelector('.w-chat-container'))) === null)
        return null;

    let ws = new WebSocket(`${wshost}/stream/${stream}`);

    ws.onopen = () => {
        self.view.on_open(ws);
        self.chat.on_open(ws);
    };

    ws.onerror = (ev) => {
        self.view.on_error();
        self.chat.on_error();
    };

    ws.onclose = (ev) => {
        self.view.on_close();
        self.chat.on_close();
    };

    ws.onmessage = (ev) => {
        // TODO
        self.chat.on_message(ev.data);
    };

    return self;
};


wcast_stream_view_init(document.body,
    `ws${window.location.protocol == 'https:' ? 's' : ''}://${window.location.host}`);
