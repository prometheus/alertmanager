/*

import Maybe exposing (Just, Nothing)

*/

function _Url_percentEncode(string)
{
	return encodeURIComponent(string);
}

function _Url_percentDecode(string)
{
	try
	{
		return __Maybe_Just(decodeURIComponent(string));
	}
	catch (e)
	{
		return __Maybe_Nothing;
	}
}