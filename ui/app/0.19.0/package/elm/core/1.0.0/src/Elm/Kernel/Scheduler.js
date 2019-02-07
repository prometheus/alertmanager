/*

import Elm.Kernel.Utils exposing (Tuple0)

*/


// TASKS

function _Scheduler_succeed(value)
{
	return {
		$: __1_SUCCEED,
		__value: value
	};
}

function _Scheduler_fail(error)
{
	return {
		$: __1_FAIL,
		__value: error
	};
}

function _Scheduler_binding(callback)
{
	return {
		$: __1_BINDING,
		__callback: callback,
		__kill: null
	};
}

var _Scheduler_andThen = F2(function(callback, task)
{
	return {
		$: __1_AND_THEN,
		__callback: callback,
		__task: task
	};
});

var _Scheduler_onError = F2(function(callback, task)
{
	return {
		$: __1_ON_ERROR,
		__callback: callback,
		__task: task
	};
});

function _Scheduler_receive(callback)
{
	return {
		$: __1_RECEIVE,
		__callback: callback
	};
}


// PROCESSES

var _Scheduler_guid = 0;

function _Scheduler_rawSpawn(task)
{
	var proc = {
		$: __2_PROCESS,
		__id: _Scheduler_guid++,
		__root: task,
		__stack: null,
		__mailbox: []
	};

	_Scheduler_enqueue(proc);

	return proc;
}

function _Scheduler_spawn(task)
{
	return _Scheduler_binding(function(callback) {
		callback(_Scheduler_succeed(_Scheduler_rawSpawn(task)));
	});
}

function _Scheduler_rawSend(proc, msg)
{
	proc.__mailbox.push(msg);
	_Scheduler_enqueue(proc);
}

var _Scheduler_send = F2(function(proc, msg)
{
	return _Scheduler_binding(function(callback) {
		_Scheduler_rawSend(proc, msg);
		callback(_Scheduler_succeed(__Utils_Tuple0));
	});
});

function _Scheduler_kill(proc)
{
	return _Scheduler_binding(function(callback) {
		var task = proc.__root;
		if (task.$ === __1_BINDING && task.__kill)
		{
			task.__kill();
		}

		proc.__root = null;

		callback(_Scheduler_succeed(__Utils_Tuple0));
	});
}


/* STEP PROCESSES

type alias Process =
  { $ : tag
  , id : unique_id
  , root : Task
  , stack : null | { $: SUCCEED | FAIL, a: callback, b: stack }
  , mailbox : [msg]
  }

*/


var _Scheduler_working = false;
var _Scheduler_queue = [];


function _Scheduler_enqueue(proc)
{
	_Scheduler_queue.push(proc);
	if (_Scheduler_working)
	{
		return;
	}
	_Scheduler_working = true;
	while (proc = _Scheduler_queue.shift())
	{
		_Scheduler_step(proc);
	}
	_Scheduler_working = false;
}


function _Scheduler_step(proc)
{
	while (proc.__root)
	{
		var rootTag = proc.__root.$;
		if (rootTag === __1_SUCCEED || rootTag === __1_FAIL)
		{
			while (proc.__stack && proc.__stack.$ !== rootTag)
			{
				proc.__stack = proc.__stack.__rest;
			}
			if (!proc.__stack)
			{
				return;
			}
			proc.__root = proc.__stack.__callback(proc.__root.__value);
			proc.__stack = proc.__stack.__rest;
		}
		else if (rootTag === __1_BINDING)
		{
			proc.__root.__kill = proc.__root.__callback(function(newRoot) {
				proc.__root = newRoot;
				_Scheduler_enqueue(proc);
			});
			return;
		}
		else if (rootTag === __1_RECEIVE)
		{
			if (proc.__mailbox.length === 0)
			{
				return;
			}
			proc.__root = proc.__root.__callback(proc.__mailbox.shift());
		}
		else // if (rootTag === __1_AND_THEN || rootTag === __1_ON_ERROR)
		{
			proc.__stack = {
				$: rootTag === __1_AND_THEN ? __1_SUCCEED : __1_FAIL,
				__callback: proc.__root.__callback,
				__rest: proc.__stack
			};
			proc.__root = proc.__root.__task;
		}
	}
}
