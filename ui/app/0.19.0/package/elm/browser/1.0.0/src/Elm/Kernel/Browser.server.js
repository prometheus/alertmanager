/*

import Elm.Kernel.Debug exposing (crash)
import Elm.Kernel.Json exposing (run, wrap)
import Elm.Kernel.Platform exposing (preload)
import Elm.Kernel.Scheduler exposing (binding, succeed, spawn)
import Elm.Kernel.Utils exposing (Tuple0)
import Elm.Kernel.VirtualDom exposing (body)
import Json.Decode as Json exposing (map)
import Platform.Sub as Sub exposing (none)
import Result exposing (isOk)

*/



// DUMMY STUFF


function _Browser_invalidUrl(url) { __Debug_crash(1, url); }
function _Browser_makeUnitTask() { return _Browser_unitTask; }
function _Browser_makeNeverResolve() { return __Scheduler_binding(function(){}); }
var _Browser_unitTask = __Scheduler_succeed(__Utils_Tuple0);
var _Browser_go = _Browser_makeUnitTask;
var _Browser_pushState = _Browser_makeNeverResolve
var _Browser_replaceState = _Browser_makeNeverResolve;
var _Browser_reload = _Browser_makeUnitTask;
var _Browser_load = _Browser_makeUnitTask;
var _Browser_call = F2(_Browser_makeUnitTask);
var _Browser_setPositiveScroll = F3(_Browser_makeUnitTask);
var _Browser_setNegativeScroll = F4(_Browser_makeUnitTask);
var _Browser_getScroll = _Browser_makeNeverResolve;
var _Browser_on = F4(function() { return __Scheduler_spawn(_Browser_unitTask); });



// PROGRAMS


var _Browser_element = F4(function(impl, flagDecoder, object, debugMetadata)
{
	object['prerender'] = function(flags)
	{
		return _Browser_prerender(flagDecoder, flags, impl);
	};

	object['render'] = function(flags)
	{
		return _Browser_render(flagDecoder, flags, impl, function(html, preload) {
			return {
				html: html,
				preload: preload
			};
		});
	};
});


var _Browser_document = F4(function(impl, flagDecoder, object, debugMetadata)
{
	object['prerender'] = function(url, flags)
	{
		return _Browser_prerender(_Browser_addEnv(url, flagDecoder), flags, impl);
	};

	object['render'] = function(url, flags)
	{
		return _Browser_render(_Browser_addEnv(url, flagDecoder), flags, impl, function(ui, preload) {
			return {
				title: ui.__$title,
				body: __VirtualDom_body(ui.__$body),
				preload: preload
			};
		});
	};
});



// PROGRAM HELPERS


function _Browser_prerender(flagDecoder, flags, impl)
{
	__Platform_preload = new Set();
	_Browser_dispatchCommands(_Browser_init(flagDecoder, flags, impl.__$init).b);
	var preload = __Platform_preload;
	__Platform_preload = null;
	return preload;
}


function _Browser_render(flagDecoder, flags, impl, toOutput)
{
	__Platform_preload = new Set();
	var pair = _Browser_init(flagDecoder, flags, impl.__$init);
	_Browser_dispatchCommands(pair.b);
	var view = impl.__$view(pair.a);
	var preload = __Platform_preload;
	__Platform_preload = null;
	return toOutput(view, preload);
}


function _Browser_init(flagDecoder, flags, init)
{
	var result = A2(__Json_run, flagDecoder, __Json_wrap(flags));
	return __Result_isOk(result) ? init(result.a) : __Debug_crash(2, result.a);
}


function _Browser_dispatchCommands(commands)
{
	var managers = {};
	_Platform_setupEffects(managers, function() {});
	_Platform_dispatchEffects(managers, commands, __Sub_none);
}



// FULLSCREEN ENV


function _Browser_addEnv(url, flagDecoder)
{
	return A2(__Json_map, function(flags) {
		return {
			__$flags: flags,
			__$url: url
		};
	}, flagDecoder);
}
