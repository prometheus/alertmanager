/*

import Basics exposing (never)
import Browser exposing (Internal, External)
import Browser.Dom as Dom exposing (NotFound)
import Elm.Kernel.Debug exposing (crash)
import Elm.Kernel.Debugger exposing (element, document)
import Elm.Kernel.Json exposing (runHelp)
import Elm.Kernel.List exposing (Nil)
import Elm.Kernel.Platform exposing (initialize)
import Elm.Kernel.Scheduler exposing (binding, fail, rawSpawn, succeed, spawn)
import Elm.Kernel.Utils exposing (Tuple0, Tuple2)
import Elm.Kernel.VirtualDom exposing (appendChild, applyPatches, diff, doc, node, passiveSupported, render, divertHrefToApp)
import Json.Decode as Json exposing (map)
import Maybe exposing (Just, Nothing)
import Result exposing (isOk)
import Task exposing (perform)
import Url exposing (fromString)

*/



// ELEMENT


var __Debugger_element;

var _Browser_element = __Debugger_element || F4(function(impl, flagDecoder, debugMetadata, args)
{
	return __Platform_initialize(
		flagDecoder,
		args,
		impl.__$init,
		impl.__$update,
		impl.__$subscriptions,
		function(sendToApp, initialModel) {
			var view = impl.__$view;
			/**__PROD/
			var domNode = args['node'];
			//*/
			/**__DEBUG/
			var domNode = args && args['node'] ? args['node'] : __Debug_crash(0);
			//*/
			var currNode = _VirtualDom_virtualize(domNode);

			return _Browser_makeAnimator(initialModel, function(model)
			{
				var nextNode = view(model);
				var patches = __VirtualDom_diff(currNode, nextNode);
				domNode = __VirtualDom_applyPatches(domNode, currNode, patches, sendToApp);
				currNode = nextNode;
			});
		}
	);
});



// DOCUMENT


var __Debugger_document;

var _Browser_document = __Debugger_document || F4(function(impl, flagDecoder, debugMetadata, args)
{
	return __Platform_initialize(
		flagDecoder,
		args,
		impl.__$init,
		impl.__$update,
		impl.__$subscriptions,
		function(sendToApp, initialModel) {
			var divertHrefToApp = impl.__$setup && impl.__$setup(sendToApp)
			var view = impl.__$view;
			var title = __VirtualDom_doc.title;
			var bodyNode = __VirtualDom_doc.body;
			var currNode = _VirtualDom_virtualize(bodyNode);
			return _Browser_makeAnimator(initialModel, function(model)
			{
				__VirtualDom_divertHrefToApp = divertHrefToApp;
				var doc = view(model);
				var nextNode = __VirtualDom_node('body')(__List_Nil)(doc.__$body);
				var patches = __VirtualDom_diff(currNode, nextNode);
				bodyNode = __VirtualDom_applyPatches(bodyNode, currNode, patches, sendToApp);
				currNode = nextNode;
				__VirtualDom_divertHrefToApp = 0;
				(title !== doc.__$title) && (__VirtualDom_doc.title = title = doc.__$title);
			});
		}
	);
});



// ANIMATION


var _Browser_requestAnimationFrame =
	typeof requestAnimationFrame !== 'undefined'
		? requestAnimationFrame
		: function(callback) { setTimeout(callback, 1000 / 60); };


function _Browser_makeAnimator(model, draw)
{
	draw(model);

	var state = __4_NO_REQUEST;

	function updateIfNeeded()
	{
		state = state === __4_EXTRA_REQUEST
			? __4_NO_REQUEST
			: ( _Browser_requestAnimationFrame(updateIfNeeded), draw(model), __4_EXTRA_REQUEST );
	}

	return function(nextModel, isSync)
	{
		model = nextModel;

		isSync
			? ( draw(model),
				state === __4_PENDING_REQUEST && (state = __4_EXTRA_REQUEST)
				)
			: ( state === __4_NO_REQUEST && _Browser_requestAnimationFrame(updateIfNeeded),
				state = __4_PENDING_REQUEST
				);
	};
}



// APPLICATION


function _Browser_application(impl)
{
	var onUrlChange = impl.__$onUrlChange;
	var onUrlRequest = impl.__$onUrlRequest;
	var key = function() { key.__sendToApp(onUrlChange(_Browser_getUrl())); };

	return _Browser_document({
		__$setup: function(sendToApp)
		{
			key.__sendToApp = sendToApp;
			_Browser_window.addEventListener('popstate', key);
			_Browser_window.navigator.userAgent.indexOf('Trident') < 0 || _Browser_window.addEventListener('hashchange', key);

			return F2(function(domNode, event)
			{
				if (!event.ctrlKey && !event.metaKey && !event.shiftKey && event.button < 1 && !domNode.target && !domNode.download)
				{
					event.preventDefault();
					var href = domNode.href;
					var curr = _Browser_getUrl();
					var next = __Url_fromString(href).a;
					sendToApp(onUrlRequest(
						(next
							&& curr.__$protocol === next.__$protocol
							&& curr.__$host === next.__$host
							&& curr.__$port_.a === next.__$port_.a
						)
							? __Browser_Internal(next)
							: __Browser_External(href)
					));
				}
			});
		},
		__$init: function(flags)
		{
			return A3(impl.__$init, flags, _Browser_getUrl(), key);
		},
		__$view: impl.__$view,
		__$update: impl.__$update,
		__$subscriptions: impl.__$subscriptions
	});
}

function _Browser_getUrl()
{
	return __Url_fromString(__VirtualDom_doc.location.href).a || __Debug_crash(1);
}

var _Browser_go = F2(function(key, n)
{
	return A2(__Task_perform, __Basics_never, __Scheduler_binding(function() {
		n && history.go(n);
		key();
	}));
});

var _Browser_pushUrl = F2(function(key, url)
{
	return A2(__Task_perform, __Basics_never, __Scheduler_binding(function() {
		history.pushState({}, '', url);
		key();
	}));
});

var _Browser_replaceUrl = F2(function(key, url)
{
	return A2(__Task_perform, __Basics_never, __Scheduler_binding(function() {
		history.replaceState({}, '', url);
		key();
	}));
});



// GLOBAL EVENTS


var _Browser_fakeNode = { addEventListener: function() {}, removeEventListener: function() {} };
var _Browser_doc = typeof document !== 'undefined' ? document : _Browser_fakeNode;
var _Browser_window = typeof window !== 'undefined' ? window : _Browser_fakeNode;

var _Browser_on = F3(function(node, eventName, sendToSelf)
{
	return __Scheduler_spawn(__Scheduler_binding(function(callback)
	{
		function handler(event)	{ __Scheduler_rawSpawn(sendToSelf(event)); }
		node.addEventListener(eventName, handler, __VirtualDom_passiveSupported && { passive: true });
		return function() { node.removeEventListener(eventName, handler); };
	}));
});

var _Browser_decodeEvent = F2(function(decoder, event)
{
	var result = __Json_runHelp(decoder, event);
	return __Result_isOk(result) ? __Maybe_Just(result.a) : __Maybe_Nothing;
});



// PAGE VISIBILITY


function _Browser_visibilityInfo()
{
	return (typeof __VirtualDom_doc.hidden !== 'undefined')
		? { __$hidden: 'hidden', __$change: 'visibilitychange' }
		:
	(typeof __VirtualDom_doc.mozHidden !== 'undefined')
		? { __$hidden: 'mozHidden', __$change: 'mozvisibilitychange' }
		:
	(typeof __VirtualDom_doc.msHidden !== 'undefined')
		? { __$hidden: 'msHidden', __$change: 'msvisibilitychange' }
		:
	(typeof __VirtualDom_doc.webkitHidden !== 'undefined')
		? { __$hidden: 'webkitHidden', __$change: 'webkitvisibilitychange' }
		: { __$hidden: 'hidden', __$change: 'visibilitychange' };
}



// ANIMATION FRAMES


function _Browser_rAF()
{
	return __Scheduler_binding(function(callback)
	{
		var id = requestAnimationFrame(function() {
			callback(__Scheduler_succeed(Date.now()));
		});

		return function() {
			cancelAnimationFrame(id);
		};
	});
}


function _Browser_now()
{
	return __Scheduler_binding(function(callback)
	{
		callback(__Scheduler_succeed(Date.now()));
	});
}



// DOM STUFF


function _Browser_withNode(id, doStuff)
{
	return __Scheduler_binding(function(callback)
	{
		_Browser_requestAnimationFrame(function() {
			var node = document.getElementById(id);
			callback(node
				? __Scheduler_succeed(doStuff(node))
				: __Scheduler_fail(__Dom_NotFound(id))
			);
		});
	});
}


function _Browser_withWindow(doStuff)
{
	return __Scheduler_binding(function(callback)
	{
		_Browser_requestAnimationFrame(function() {
			callback(__Scheduler_succeed(doStuff()));
		});
	});
}


// FOCUS and BLUR


var _Browser_call = F2(function(functionName, id)
{
	return _Browser_withNode(id, function(node) {
		node[functionName]();
		return __Utils_Tuple0;
	});
});



// WINDOW VIEWPORT


function _Browser_getViewport()
{
	return {
		__$scene: _Browser_getScene(),
		__$viewport: {
			__$x: _Browser_window.pageXOffset,
			__$y: _Browser_window.pageYOffset,
			__$width: _Browser_doc.documentElement.clientWidth,
			__$height: _Browser_doc.documentElement.clientHeight
		}
	};
}

function _Browser_getScene()
{
	var body = _Browser_doc.body;
	var elem = _Browser_doc.documentElement;
	return {
		__$width: Math.max(body.scrollWidth, body.offsetWidth, elem.scrollWidth, elem.offsetWidth, elem.clientWidth),
		__$height: Math.max(body.scrollHeight, body.offsetHeight, elem.scrollHeight, elem.offsetHeight, elem.clientHeight)
	};
}

var _Browser_setViewport = F2(function(x, y)
{
	return _Browser_withWindow(function()
	{
		_Browser_window.scroll(x, y);
		return __Utils_Tuple0;
	});
});



// ELEMENT VIEWPORT


function _Browser_getViewportOf(id)
{
	return _Browser_withNode(id, function(node)
	{
		return {
			__$scene: {
				__$width: node.scrollWidth,
				__$height: node.scrollHeight
			},
			__$viewport: {
				__$x: node.scrollLeft,
				__$y: node.scrollTop,
				__$width: node.clientWidth,
				__$height: node.clientHeight
			}
		};
	});
}


var _Browser_setViewportOf = F3(function(id, x, y)
{
	return _Browser_withNode(id, function(node)
	{
		node.scrollLeft = x;
		node.scrollTop = y;
		return __Utils_Tuple0;
	});
});



// ELEMENT


function _Browser_getElement(id)
{
	return _Browser_withNode(id, function(node)
	{
		var rect = node.getBoundingClientRect();
		var x = _Browser_window.pageXOffset;
		var y = _Browser_window.pageYOffset;
		return {
			__$scene: _Browser_getScene(),
			__$viewport: {
				__$x: x,
				__$y: y,
				__$width: _Browser_doc.documentElement.clientWidth,
				__$height: _Browser_doc.documentElement.clientHeight
			},
			__$element: {
				__$x: x + rect.left,
				__$y: y + rect.top,
				__$width: rect.width,
				__$height: rect.height
			}
		};
	});
}



// LOAD and RELOAD


function _Browser_reload(skipCache)
{
	return A2(__Task_perform, __Basics_never, __Scheduler_binding(function(callback)
	{
		__VirtualDom_doc.location.reload(skipCache);
	}));
}

function _Browser_load(url)
{
	return A2(__Task_perform, __Basics_never, __Scheduler_binding(function(callback)
	{
		try
		{
			_Browser_window.location = url;
		}
		catch(err)
		{
			// Only Firefox can throw a NS_ERROR_MALFORMED_URI exception here.
			// Other browsers reload the page, so let's be consistent about that.
			__VirtualDom_doc.location.reload(false);
		}
	}));
}
