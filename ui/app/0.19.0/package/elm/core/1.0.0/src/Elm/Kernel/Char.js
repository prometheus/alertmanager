/*

import Elm.Kernel.Utils exposing (chr)

*/


function _Char_toCode(char)
{
	var code = char.charCodeAt(0);
	if (0xD800 <= code && code <= 0xDBFF)
	{
		return (code - 0xD800) * 0x400 + char.charCodeAt(1) - 0xDC00 + 0x10000
	}
	return code;
}

function _Char_fromCode(code)
{
	return __Utils_chr(
		(code < 0 || 0x10FFFF < code)
			? '\uFFFD'
			:
		(code <= 0xFFFF)
			? String.fromCharCode(code)
			:
		(code -= 0x10000,
			String.fromCharCode(Math.floor(code / 0x400) + 0xD800)
			+
			String.fromCharCode(code % 0x400 + 0xDC00)
		)
	);
}

function _Char_toUpper(char)
{
	return __Utils_chr(char.toUpperCase());
}

function _Char_toLower(char)
{
	return __Utils_chr(char.toLowerCase());
}

function _Char_toLocaleUpper(char)
{
	return __Utils_chr(char.toLocaleUpperCase());
}

function _Char_toLocaleLower(char)
{
	return __Utils_chr(char.toLocaleLowerCase());
}
