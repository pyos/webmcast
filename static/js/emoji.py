#!/usr/bin/env python3
'''
Emoji database generator. Output format, where all arrays are null-terminated and strings are UTF-8:

    struct { u8 name[]; struct { u8 key[]; u8 sequence[]; } emoji[]; } categories[];

'''
import re
import sys
import itertools
import collections
import urllib.request


KEYWORDS = 'http://unicode.org/emoji/charts/emoji-annotations.html'
KEYWORDS_TOKENS = re.compile("(?s)"
    r"(?P<break></tr>)|"
    r"(?P<keyword><a href='#.+?' name='.+?'>(?P<kwdname>.+?)</a>)|"
    r"(?P<character><a class='plain' href='.+?#(?P<codepoints>[0-9a-f]+(?:_[0-9a-f]+)*)' target='full'>)"
)

ORDERING = 'http://unicode.org/emoji/charts/emoji-ordering.html'
ORDERING_TOKENS = re.compile("(?s)"
    r"(?P<category><th class='bighead'><a .+?>(?P<catname>.*?)</a></th>)|"
    r"(?P<character><a class='plain' href='.+?#(?P<codepoints>[0-9a-f]+(?:_[0-9a-f]+)*)' target='full'>)"
)


def fetch(url):
    name = url.rsplit('/', 1)[-1]
    try:
        with open(name, 'rb') as fd:
            data = fd.read()
        print('# \033[1;32mcached\033[0m', url, '->', name, file=sys.stderr)
    except FileNotFoundError:
        print('# \033[1;34msaving\033[0m', url, '->', name, file=sys.stderr)
        with urllib.request.urlopen(url) as fd:
            data = fd.read()
        with open(name, 'wb') as fd:
            fd.write(data)
    return data.decode('utf-8')


def character(codepoints: '1234_5678_90ab_cdef') -> '\u1234\u5678\u90ab\ucdef':
    return ''.join(chr(int(u, 16)) for u in codepoints.split('_'))


def load_names():
    kwds_of    = collections.defaultdict(set)
    chars_with = collections.defaultdict(set)

    kwds = set()
    chrs = set()
    for token in KEYWORDS_TOKENS.finditer(fetch(KEYWORDS)):
        if token.lastgroup == 'break':
            for k in kwds:
                chars_with[k] |= chrs
            for c in chrs:
                kwds_of[c] |= kwds
            kwds = set()
            chrs = set()
        elif token.lastgroup == 'keyword':
            kwds.add(token.group('kwdname'))
        else:
            chrs.add(character(token.group('codepoints')))
    print('# \033[1;32mloaded\033[0m', len(kwds_of), 'emoji,', len(chars_with), 'keywords', file=sys.stderr)

    names = {}
    ordered = sorted(chars_with.items())
    while True:
        for k, chars in ordered:
            if len(chars) == 1:
                c, = chars
                names[c] = k
                for k2 in kwds_of[c]:
                    chars_with[k2].discard(c)
                break
        else:
            break
    print('# \033[1;33mno unique names\033[0m for', len(kwds_of) - len(names), 'emoji', file=sys.stderr)
    return names


def load_categories():
    cats = collections.OrderedDict()
    last = 'Other'
    for token in ORDERING_TOKENS.finditer(fetch(ORDERING)):
        if token.lastgroup == 'category':
            last = token.group('catname')
        else:
            cats[character(token.group('codepoints'))] = last
    for cat, count in collections.Counter(cats.values()).most_common():
        print('# \033[1;32mcategory\033[0m', cat.ljust(20, ' '), count, file=sys.stderr)
    return cats


def format(names, categories):
    cats = []
    for cat in categories.values():
        if cat not in cats:
            cats.append(cat)
    for cat, chars in itertools.groupby(sorted(categories, key=lambda u: cats.index(categories[u])), key=categories.__getitem__):
        yield cat.encode('utf-8') + b'\0'
        for char in chars:
            yield names.get(char, char).encode('utf-8') + b'\0'
            yield char.encode('utf-8') + b'\0'
        yield b'\0'
    yield b'\0'


if __name__ == '__main__':
    sys.stdout.buffer.write(b''.join(format(load_names(), load_categories())))
