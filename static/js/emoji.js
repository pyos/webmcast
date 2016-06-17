"use strict";  /* global $, twemoji */

$.emoji = () => {
    let root = document.createElement('x-tooltip');
    let wrap = document.createElement('div');
    wrap.classList.add('emoji-selector');
    wrap.dataset.status = 'loading';
    root.appendChild(wrap);
    $.emoji.load()
        .then(_ => {
            delete wrap.dataset.status;
            $.emoji.makeSelector(wrap);
        })
        .catch(_ => wrap.dataset.status = 'error');
    return root;
};

// stolen from twemoji. https://github.com/twitter/twemoji/blob/gh-pages/2/twemoji.js
$.emoji.re = /\ud83d\udc68\u200d\u2764\ufe0f\u200d\ud83d\udc8b\u200d\ud83d\udc68|\ud83d\udc68\u200d\ud83d\udc68\u200d\ud83d\udc66\u200d\ud83d\udc66|\ud83d\udc68\u200d\ud83d\udc68\u200d\ud83d\udc67\u200d\ud83d[\udc66\udc67]|\ud83d\udc68\u200d\ud83d\udc69\u200d\ud83d\udc66\u200d\ud83d\udc66|\ud83d\udc68\u200d\ud83d\udc69\u200d\ud83d\udc67\u200d\ud83d[\udc66\udc67]|\ud83d\udc69\u200d\u2764\ufe0f\u200d\ud83d\udc8b\u200d\ud83d[\udc68\udc69]|\ud83d\udc69\u200d\ud83d\udc69\u200d\ud83d\udc66\u200d\ud83d\udc66|\ud83d\udc69\u200d\ud83d\udc69\u200d\ud83d\udc67\u200d\ud83d[\udc66\udc67]|\ud83d\udc68\u200d\u2764\ufe0f\u200d\ud83d\udc68|\ud83d\udc68\u200d\ud83d\udc68\u200d\ud83d[\udc66\udc67]|\ud83d\udc68\u200d\ud83d\udc69\u200d\ud83d[\udc66\udc67]|\ud83d\udc69\u200d\u2764\ufe0f\u200d\ud83d[\udc68\udc69]|\ud83d\udc69\u200d\ud83d\udc69\u200d\ud83d[\udc66\udc67]|\ud83d\udc41\u200d\ud83d\udde8|(?:[\u0023\u002a\u0030-\u0039])\ufe0f?\u20e3|(?:(?:[\u261d\u270c])(?:\ufe0f|(?!\ufe0e))|\ud83c[\udf85\udfc2-\udfc4\udfc7\udfca\udfcb]|\ud83d[\udc42\udc43\udc46-\udc50\udc66-\udc69\udc6e\udc70-\udc78\udc7c\udc81-\udc83\udc85-\udc87\udcaa\udd75\udd90\udd95\udd96\ude45-\ude47\ude4b-\ude4f\udea3\udeb4-\udeb6\udec0]|\ud83e\udd18|[\u26f9\u270a\u270b\u270d])(?:\ud83c[\udffb-\udfff]|)|\ud83c\udde6\ud83c[\udde8-\uddec\uddee\uddf1\uddf2\uddf4\uddf6-\uddfa\uddfc\uddfd\uddff]|\ud83c\udde7\ud83c[\udde6\udde7\udde9-\uddef\uddf1-\uddf4\uddf6-\uddf9\uddfb\uddfc\uddfe\uddff]|\ud83c\udde8\ud83c[\udde6\udde8\udde9\uddeb-\uddee\uddf0-\uddf5\uddf7\uddfa-\uddff]|\ud83c\udde9\ud83c[\uddea\uddec\uddef\uddf0\uddf2\uddf4\uddff]|\ud83c\uddea\ud83c[\udde6\udde8\uddea\uddec\udded\uddf7-\uddfa]|\ud83c\uddeb\ud83c[\uddee-\uddf0\uddf2\uddf4\uddf7]|\ud83c\uddec\ud83c[\udde6\udde7\udde9-\uddee\uddf1-\uddf3\uddf5-\uddfa\uddfc\uddfe]|\ud83c\udded\ud83c[\uddf0\uddf2\uddf3\uddf7\uddf9\uddfa]|\ud83c\uddee\ud83c[\udde8-\uddea\uddf1-\uddf4\uddf6-\uddf9]|\ud83c\uddef\ud83c[\uddea\uddf2\uddf4\uddf5]|\ud83c\uddf0\ud83c[\uddea\uddec-\uddee\uddf2\uddf3\uddf5\uddf7\uddfc\uddfe\uddff]|\ud83c\uddf1\ud83c[\udde6-\udde8\uddee\uddf0\uddf7-\uddfb\uddfe]|\ud83c\uddf2\ud83c[\udde6\udde8-\udded\uddf0-\uddff]|\ud83c\uddf3\ud83c[\udde6\udde8\uddea-\uddec\uddee\uddf1\uddf4\uddf5\uddf7\uddfa\uddff]|\ud83c\uddf4\ud83c\uddf2|\ud83c\uddf5\ud83c[\udde6\uddea-\udded\uddf0-\uddf3\uddf7-\uddf9\uddfc\uddfe]|\ud83c\uddf6\ud83c\udde6|\ud83c\uddf7\ud83c[\uddea\uddf4\uddf8\uddfa\uddfc]|\ud83c\uddf8\ud83c[\udde6-\uddea\uddec-\uddf4\uddf7-\uddf9\uddfb\uddfd-\uddff]|\ud83c\uddf9\ud83c[\udde6\udde8\udde9\uddeb-\udded\uddef-\uddf4\uddf7\uddf9\uddfb\uddfc\uddff]|\ud83c\uddfa\ud83c[\udde6\uddec\uddf2\uddf8\uddfe\uddff]|\ud83c\uddfb\ud83c[\udde6\udde8\uddea\uddec\uddee\uddf3\uddfa]|\ud83c\uddfc\ud83c[\uddeb\uddf8]|\ud83c\uddfd\ud83c\uddf0|\ud83c\uddfe\ud83c[\uddea\uddf9]|\ud83c\uddff\ud83c[\udde6\uddf2\uddfc]|\ud83c[\udccf\udd8e\udd91-\udd9a\udde6-\uddff\ude01\ude32-\ude36\ude38-\ude3a\ude50\ude51\udf00-\udf21\udf24-\udf84\udf86-\udf93\udf96\udf97\udf99-\udf9b\udf9e-\udfc1\udfc5\udfc6\udfc8\udfc9\udfcc-\udff0\udff3-\udff5\udff7-\udfff]|\ud83d[\udc00-\udc41\udc44\udc45\udc51-\udc65\udc6a-\udc6d\udc6f\udc79-\udc7b\udc7d-\udc80\udc84\udc88-\udca9\udcab-\udcfd\udcff-\udd3d\udd49-\udd4e\udd50-\udd67\udd6f\udd70\udd73\udd74\udd76-\udd79\udd87\udd8a-\udd8d\udda5\udda8\uddb1\uddb2\uddbc\uddc2-\uddc4\uddd1-\uddd3\udddc-\uddde\udde1\udde3\udde8\uddef\uddf3\uddfa-\ude44\ude48-\ude4a\ude80-\udea2\udea4-\udeb3\udeb7-\udebf\udec1-\udec5\udecb-\uded0\udee0-\udee5\udee9\udeeb\udeec\udef0\udef3]|\ud83e[\udd10-\udd17\udd80-\udd84\uddc0]|[\u2328\u23cf\u23e9-\u23f3\u23f8-\u23fa\u2602-\u2604\u2618\u2620\u2622\u2623\u2626\u262a\u262e\u262f\u2638\u2692\u2694\u2696\u2697\u2699\u269b\u269c\u26b0\u26b1\u26c8\u26ce\u26cf\u26d1\u26d3\u26e9\u26f0\u26f1\u26f4\u26f7\u26f8\u2705\u271d\u2721\u2728\u274c\u274e\u2753-\u2755\u2763\u2795-\u2797\u27b0\u27bf\ue50a]|(?:\ud83c[\udc04\udd70\udd71\udd7e\udd7f\ude02\ude1a\ude2f\ude37]|[\u00a9\u00ae\u203c\u2049\u2122\u2139\u2194-\u2199\u21a9\u21aa\u231a\u231b\u24c2\u25aa\u25ab\u25b6\u25c0\u25fb-\u25fe\u2600\u2601\u260e\u2611\u2614\u2615\u2639\u263a\u2648-\u2653\u2660\u2663\u2665\u2666\u2668\u267b\u267f\u2693\u26a0\u26a1\u26aa\u26ab\u26bd\u26be\u26c4\u26c5\u26d4\u26ea\u26f2\u26f3\u26f5\u26fa\u26fd\u2702\u2708\u2709\u270f\u2712\u2714\u2716\u2733\u2734\u2744\u2747\u2757\u2764\u27a1\u2934\u2935\u2b05-\u2b07\u2b1b\u2b1c\u2b50\u2b55\u3030\u303d\u3297\u3299])(?:\ufe0f|(?!\ufe0e))/g;

// also basically stolen from twemoji. I don't need the rest of its code.
$.emoji.parse = text => text.replace($.emoji.re, m => {
    if (m.indexOf('\u200d') === -1)
        m = m.replace('\ufe0f', '');  // "prefer image" variant marker
    let r = [];
    for (let i = 0; i < m.length; i += 1 + (m.charCodeAt(i) != m.codePointAt(i)))
        r.push(m.codePointAt(i).toString(16));
    return `<img class="emoji" draggable="false" alt="${m}" src="https://twemoji.maxcdn.com/2/72x72/${r.join('-')}.png">`;
});

// NOT stolen from twemoji.
$.emoji.categories = [] /* [{name: ..., emoji: sequence}] */;

$.emoji.load = () => new Promise((resolve, reject) => {
    if ($.emoji.categories.length !== 0)
        return resolve(null);

    let xhr = new XMLHttpRequest();
    xhr.onload = xhr.onerror = ev => {
        if (!xhr.response || xhr.status >= 400)
            return reject(xhr);
        // struct { u8 name[]; struct { u8 key[]; u8 sequence[]; } emoji[]; } categories[];
        // arrays are null-terminated.
        let arr = new Uint8Array(xhr.response), i = 0;
        let dec = new TextDecoder('utf-8');
        let readString = () => {
            let j = i; while (arr[i++]);
            return dec.decode(new DataView(arr.buffer, j, i - j - 1));
        };
        for (; arr[i]; i++) {
            let cat = $.emoji.categories[$.emoji.categories.length] = {name: readString(), emoji: []};
            while (arr[i]) {
                let key = readString(), sequence = readString();
                if ($.emoji.re.lastIndex = 0, $.emoji.re.test(sequence))
                    cat.emoji.push(sequence);  // TODO use the key
            }
        }
        resolve(null);
    };
    xhr.responseType = 'arraybuffer';
    xhr.open('GET', '/static/js/emoji.bin');
    xhr.send();
});


$.emoji.makeSelector = root => {
    let tabs = document.createElement('div');
    tabs.dataset.tabs = '';
    tabs.addEventListener('click', ev => {
        if (!ev.target.classList.contains('emoji'))
            return;
        ev.preventDefault();
        root.dispatchEvent(new CustomEvent('select', {detail: ev.target.getAttribute('alt'), bubbles: true}));
    });
    for (let cat of $.emoji.categories) {
        let list = document.createElement('div');
        let group = document.createElement('div');
        new MutationObserver((cat => _ => {
            if (list.hasAttribute('hidden'))
                group.innerHTML = '';
            else
                group.innerHTML = $.emoji.parse(cat.emoji.join(''));
        })(cat)).observe(list, {attributes: true, attributeFilter: ['hidden']});
        list.setAttribute('hidden', '');
        list.dataset.tab = cat.emoji[0];
        list.dataset.scrollbar = '';
        list.appendChild(group);
        tabs.appendChild(list);
    }
    root.appendChild($.init(tabs));
};

$.emoji.test = () => {
    let e = $.emoji();
    e.style.bottom = 'calc(50% - 6.75em)';
    e.style.left   = 'calc(50% - 9em)';
    document.body.appendChild(e);
};