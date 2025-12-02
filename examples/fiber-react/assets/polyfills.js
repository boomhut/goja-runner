(function (global) {
  if (typeof global.TextEncoder === "function" && typeof global.TextDecoder === "function") {
    return;
  }

  function TextEncoder() {}
  TextEncoder.prototype.encode = function (str) {
    var utf8 = [];
    for (var i = 0; i < str.length; i++) {
      var charcode = str.charCodeAt(i);
      if (charcode < 0x80) utf8.push(charcode);
      else if (charcode < 0x800) {
        utf8.push(0xc0 | (charcode >> 6));
        utf8.push(0x80 | (charcode & 0x3f));
      } else if (charcode < 0xd800 || charcode >= 0xe000) {
        utf8.push(0xe0 | (charcode >> 12));
        utf8.push(0x80 | ((charcode >> 6) & 0x3f));
        utf8.push(0x80 | (charcode & 0x3f));
      } else {
        i++;
        charcode = 0x10000 + (((charcode & 0x3ff) << 10) | (str.charCodeAt(i) & 0x3ff));
        utf8.push(0xf0 | (charcode >> 18));
        utf8.push(0x80 | ((charcode >> 12) & 0x3f));
        utf8.push(0x80 | ((charcode >> 6) & 0x3f));
        utf8.push(0x80 | (charcode & 0x3f));
      }
    }
    return new Uint8Array(utf8);
  };

  function TextDecoder() {}
  TextDecoder.prototype.decode = function (bytes) {
    if (!bytes) return "";
    var arr = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
    var out = "";
    var i = 0;
    while (i < arr.length) {
      var c = arr[i++];
      if (c < 0x80) {
        out += String.fromCharCode(c);
      } else if (c < 0xe0) {
        var c2 = arr[i++];
        out += String.fromCharCode(((c & 0x1f) << 6) | (c2 & 0x3f));
      } else if (c < 0xf0) {
        var c2b = arr[i++];
        var c3 = arr[i++];
        out += String.fromCharCode(((c & 0x0f) << 12) | ((c2b & 0x3f) << 6) | (c3 & 0x3f));
      } else {
        var c2c = arr[i++];
        var c3c = arr[i++];
        var c4 = arr[i++];
        var codepoint = ((c & 0x07) << 18) | ((c2c & 0x3f) << 12) | ((c3c & 0x3f) << 6) | (c4 & 0x3f);
        codepoint -= 0x10000;
        out += String.fromCharCode((codepoint >> 10) + 0xd800);
        out += String.fromCharCode((codepoint & 0x3ff) + 0xdc00);
      }
    }
    return out;
  };

  global.TextEncoder = global.TextEncoder || TextEncoder;
  global.TextDecoder = global.TextDecoder || TextDecoder;
})(this);
