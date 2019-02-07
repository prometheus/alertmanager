/*

import Elm.Kernel.Scheduler exposing (binding, succeed)
import Elm.Kernel.Utils exposing (Tuple0)

*/


function _Process_sleep(time)
{
	return __Scheduler_binding(function(callback) {
		var id = setTimeout(function() {
			callback(__Scheduler_succeed(__Utils_Tuple0));
		}, time);

		return function() { clearTimeout(id); };
	});
}
