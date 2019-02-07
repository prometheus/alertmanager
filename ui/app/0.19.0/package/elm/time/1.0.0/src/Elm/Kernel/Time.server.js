/*

import Elm.Kernel.Scheduler exposing (binding)

*/


function _Time_neverResolve()
{
	return __Scheduler_binding(function() {});
}

var _Time_now = _Time_neverResolve;
var _Time_here = _Time_neverResolve;
var _Time_getZoneName = _Time_neverResolve;
var _Time_setInterval = F2(_Time_neverResolve);
