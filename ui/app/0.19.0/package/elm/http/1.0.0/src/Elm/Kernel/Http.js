/*

import Dict exposing (empty, update)
import Elm.Kernel.Scheduler exposing (binding, fail, rawSpawn, succeed)
import Http exposing (BadUrl, Timeout, NetworkError, BadStatus, BadPayload)
import Http.Internal as HI exposing (FormDataBody, isStringBody)
import Maybe exposing (Just, isJust)
import Result exposing (map, isOk)

*/


// SEND REQUEST

var _Http_toTask = F2(function(request, maybeProgress)
{
	return __Scheduler_binding(function(callback)
	{
		var xhr = new XMLHttpRequest();

		_Http_configureProgress(xhr, maybeProgress);

		xhr.addEventListener('error', function() {
			callback(__Scheduler_fail(__Http_NetworkError));
		});
		xhr.addEventListener('timeout', function() {
			callback(__Scheduler_fail(__Http_Timeout));
		});
		xhr.addEventListener('load', function() {
			callback(_Http_handleResponse(xhr, request.__$expect.__responseToResult));
		});

		try
		{
			xhr.open(request.__$method, request.__$url, true);
		}
		catch (e)
		{
			return callback(__Scheduler_fail(__Http_BadUrl(request.__$url)));
		}

		_Http_configureRequest(xhr, request);

		var body = request.__$body;
		xhr.send(__HI_isStringBody(body)
			? (xhr.setRequestHeader('Content-Type', body.a), body.b)
			: body.a
		);

		return function() { xhr.abort(); };
	});
});

function _Http_configureProgress(xhr, maybeProgress)
{
	if (!__Maybe_isJust(maybeProgress))
	{
		return;
	}

	xhr.addEventListener('progress', function(event) {
		if (!event.lengthComputable)
		{
			return;
		}
		__Scheduler_rawSpawn(maybeProgress.a({
			__$bytes: event.loaded,
			__$bytesExpected: event.total
		}));
	});
}

function _Http_configureRequest(xhr, request)
{
	for (var headers = request.__$headers; headers.b; headers = headers.b) // WHILE_CONS
	{
		xhr.setRequestHeader(headers.a.a, headers.a.b);
	}

	xhr.responseType = request.__$expect.__responseType;
	xhr.withCredentials = request.__$withCredentials;

	__Maybe_isJust(request.__$timeout) && (xhr.timeout = request.__$timeout.a);
}


// RESPONSES

function _Http_handleResponse(xhr, responseToResult)
{
	var response = _Http_toResponse(xhr);

	if (xhr.status < 200 || 300 <= xhr.status)
	{
		response.body = xhr.responseText;
		return __Scheduler_fail(__Http_BadStatus(response));
	}

	var result = responseToResult(response);

	if (__Result_isOk(result))
	{
		return __Scheduler_succeed(result.a);
	}
	else
	{
		response.body = xhr.responseText;
		return __Scheduler_fail(A2(__Http_BadPayload, result.a, response));
	}
}

function _Http_toResponse(xhr)
{
	return {
		__$url: xhr.responseURL,
		__$status: { __$code: xhr.status, __$message: xhr.statusText },
		__$headers: _Http_parseHeaders(xhr.getAllResponseHeaders()),
		__$body: xhr.response
	};
}

function _Http_parseHeaders(rawHeaders)
{
	var headers = __Dict_empty;

	if (!rawHeaders)
	{
		return headers;
	}

	var headerPairs = rawHeaders.split('\u000d\u000a');
	for (var i = headerPairs.length; i--; )
	{
		var headerPair = headerPairs[i];
		var index = headerPair.indexOf('\u003a\u0020');
		if (index > 0)
		{
			var key = headerPair.substring(0, index);
			var value = headerPair.substring(index + 2);

			headers = A3(__Dict_update, key, function(oldValue) {
				return __Maybe_Just(__Maybe_isJust(oldValue)
					? value + ', ' + oldValue.a
					: value
				);
			}, headers);
		}
	}

	return headers;
}


// EXPECTORS

function _Http_expectStringResponse(responseToResult)
{
	return {
		$: __0_EXPECT,
		__responseType: 'text',
		__responseToResult: responseToResult
	};
}

var _Http_mapExpect = F2(function(func, expect)
{
	return {
		$: __0_EXPECT,
		__responseType: expect.__responseType,
		__responseToResult: function(response) {
			var convertedResponse = expect.__responseToResult(response);
			return A2(__Result_map, func, convertedResponse);
		}
	};
});


// BODY

function _Http_multipart(parts)
{


	for (var formData = new FormData(); parts.b; parts = parts.b) // WHILE_CONS
	{
		var part = parts.a;
		formData.append(part.a, part.b);
	}

	return __HI_FormDataBody(formData);
}
