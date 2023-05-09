# TODO: Both encode and decode are broken.
def decode_content(content):
    """Decode the custom encoded content in the string CONTENT."""
    # The decoder implementation is in the file ea882fba1e85d801.js as
    # of [2023-04-30 ஞா 16:32].  It was found by searching for
    # _decode.
    keystr = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
    content = re.sub(r"[^A-Za-z0-9\+\/\=]", "", content)
    output = ""
    i = 0
    while i < len(content):
        r = keystr.find(content[i+1])
        e = keystr.find(content[i]) << 2 | r >> 4
        i += 2

        o = keystr.find(content[i])
        t = (15 & r) << 4 | o >> 2
        l = keystr.find(content[i+1])
        n = 3 & o << 6 | l
        i += 2

        output += chr(e)
        if o != 64: output += chr(t)
        if l != 64: output += chr(n)

    def charcodeat(idx):
        return ord(output[idx])

    s = ""
    i = 0

    while i < len(output):
        t = charcodeat(i)
        if t < 128:
            s += chr(t)
            i += 1
        elif t > 191 and t < 224:
            n = charcodeat(i + 1)
            s += chr((31 & t) << 6 | 63 & n)
            i += 2
        else:
            n = charcodeat(i + 1)
            r = charcodeat(i + 2)
            s += chr((15 & t) << 12 | (63 & n) << 6 | 63 & r)
            i += 3

    return output,s
    # {
    #     var e, t, n, r, o, l, output = "", i = 0;
    #                     for (input = input.replace(/[^A-Za-z0-9\+\/\=]/g, ""); i < input.length; )
    #     e = R._keyStr.indexOf(input.charAt(i++)) << 2 | (r = R._keyStr.indexOf(input.charAt(i++))) >> 4,
    #     t = (15 & r) << 4 | (o = R._keyStr.indexOf(input.charAt(i++))) >> 2,
    #     n = (3 & o) << 6 | (l = R._keyStr.indexOf(input.charAt(i++))),
    #     output += String.fromCharCode(e),
    #     64 !== o && (output += String.fromCharCode(t)),
    #     64 !== l && (output += String.fromCharCode(n));
    #                     return output = j(output)
    # }
    # j = function(e) {
    #                 for (var s = "", i = 0, t = 0, n = 0, r = 0; i < e.length; )
    #                     (t = e.charCodeAt(i)) < 128 ? (s += String.fromCharCode(t),
    #                     i++) : t > 191 && t < 224 ? (n = e.charCodeAt(i + 1),
    #                     s += String.fromCharCode((31 & t) << 6 | 63 & n),
    #                     i += 2) : (n = e.charCodeAt(i + 1),
    #                     r = e.charCodeAt(i + 2),
    #                     s += String.fromCharCode((15 & t) << 12 | (63 & n) << 6 | 63 & r),
    #                     i += 3);
    #                 return s
    #             }


def encode(input):
    e = ""
    def charcodeat(idx):
        return ord(input[idx])
    for t in range(len(input)):
        n = charcodeat(t)
        if n < 128:
            e += chr(n)
        elif n > 127 and n < 2048:
            e += chr(n >> 6 | 192)
            e += chr(63 & n | 128)
        else:
            e += chr(n >> 12 | 224)
            e += chr(n >> 6 & 63 | 128)
            e += chr(63 & n | 128)

    input = e
    output = ""
    keystr = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
    i = 0
    while i < len(input):
        e = charcodeat(i); i += 1
        r = e >> 2
        t = charcodeat(i); i += 1
        o = (3 & e) << 4 | t >> 4
        n = charcodeat(i); i += 1
        l = (15 & t) << 2 | n >> 6
        c = 63 & n
        if t == float('nan'):
            l = c = 64
        elif n == float('nan'):
            c = 64
        output += keystr[r] + keystr[o] + keystr[l] + keystr[c]

    return output

    # D = function(s) {
                    # for (var e = "", t = 0; t < s.length; t++) {
                    #     var n = s.charCodeAt(t);
                    #     n < 128 ? e += String.fromCharCode(n) : n > 127 && n < 2048 ? (e += String.fromCharCode(n >> 6 | 192),
                    #     e += String.fromCharCode(63 & n | 128)) : (e += String.fromCharCode(n >> 12 | 224),
                    #     e += String.fromCharCode(n >> 6 & 63 | 128),
                    #     e += String.fromCharCode(63 & n | 128))
                    # }
    # {
    #                     var e, t, n, r, o, l, c, output = "", i = 0;
    #                     for (input = D(input); i < input.length; )
    #                         r = (e = input.charCodeAt(i++)) >> 2,
    #                         o = (3 & e) << 4 | (t = input.charCodeAt(i++)) >> 4,
    #                         l = (15 & t) << 2 | (n = input.charCodeAt(i++)) >> 6,
    #                         c = 63 & n,
    #                         isNaN(t) ? l = c = 64 : isNaN(n) && (c = 64),
    #                         output = output + R._keyStr.charAt(r) + R._keyStr.charAt(o) + R._keyStr.charAt(l) + R._keyStr.charAt(c);
    #                     return output
    #                 }
