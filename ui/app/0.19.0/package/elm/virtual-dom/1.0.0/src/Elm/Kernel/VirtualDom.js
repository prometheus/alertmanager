/*

import Basics exposing (identity)
import Elm.Kernel.Debug exposing (crash)
import Elm.Kernel.Json exposing (equality, runHelp, unwrap)
import Elm.Kernel.List exposing (Cons, Nil)
import Elm.Kernel.Utils exposing (Tuple2)
import Elm.Kernel.Platform exposing (export)
import Json.Decode as Json exposing (map, map2, succeed)
import Result exposing (isOk)
import VirtualDom exposing (toHandlerInt)

*/



// HELPERS


var _VirtualDom_divertHrefToApp;

var _VirtualDom_doc = typeof document !== 'undefined' ? document : {};


function _VirtualDom_appendChild(parent, child)
{
	parent.appendChild(child);
}

var _VirtualDom_init = F4(function(virtualNode, flagDecoder, debugMetadata, args)
{
	// NOTE: this function needs __Platform_export available to work

	/**__PROD/
	var node = args['node'];
	//*/
	/**__DEBUG/
	var node = args && args['node'] ? args['node'] : __Debug_crash(0);
	//*/

	node.parentNode.replaceChild(
		_VirtualDom_render(virtualNode, function() {}),
		node
	);

	return {};
});



// TEXT


function _VirtualDom_text(string)
{
	return {
		$: __2_TEXT,
		__text: string
	};
}



// NODE


var _VirtualDom_nodeNS = F2(function(namespace, tag)
{
	return F2(function(factList, kidList)
	{
		for (var kids = [], descendantsCount = 0; kidList.b; kidList = kidList.b) // WHILE_CONS
		{
			var kid = kidList.a;
			descendantsCount += (kid.__descendantsCount || 0);
			kids.push(kid);
		}
		descendantsCount += kids.length;

		return {
			$: __2_NODE,
			__tag: tag,
			__facts: _VirtualDom_organizeFacts(factList),
			__kids: kids,
			__namespace: namespace,
			__descendantsCount: descendantsCount
		};
	});
});


var _VirtualDom_node = _VirtualDom_nodeNS(undefined);



// KEYED NODE


var _VirtualDom_keyedNodeNS = F2(function(namespace, tag)
{
	return F2(function(factList, kidList)
	{
		for (var kids = [], descendantsCount = 0; kidList.b; kidList = kidList.b) // WHILE_CONS
		{
			var kid = kidList.a;
			descendantsCount += (kid.b.__descendantsCount || 0);
			kids.push(kid);
		}
		descendantsCount += kids.length;

		return {
			$: __2_KEYED_NODE,
			__tag: tag,
			__facts: _VirtualDom_organizeFacts(factList),
			__kids: kids,
			__namespace: namespace,
			__descendantsCount: descendantsCount
		};
	});
});


var _VirtualDom_keyedNode = _VirtualDom_keyedNodeNS(undefined);



// CUSTOM


function _VirtualDom_custom(factList, model, render, diff)
{
	return {
		$: __2_CUSTOM,
		__facts: _VirtualDom_organizeFacts(factList),
		__model: model,
		__render: render,
		__diff: diff
	};
}



// MAP


var _VirtualDom_map = F2(function(tagger, node)
{
	return {
		$: __2_TAGGER,
		__tagger: tagger,
		__node: node,
		__descendantsCount: 1 + (node.__descendantsCount || 0)
	};
});



// LAZY


function _VirtualDom_thunk(refs, thunk)
{
	return {
		$: __2_THUNK,
		__refs: refs,
		__thunk: thunk,
		__node: undefined
	};
}

var _VirtualDom_lazy = F2(function(func, a)
{
	return _VirtualDom_thunk([func, a], function() {
		return func(a);
	});
});

var _VirtualDom_lazy2 = F3(function(func, a, b)
{
	return _VirtualDom_thunk([func, a, b], function() {
		return A2(func, a, b);
	});
});

var _VirtualDom_lazy3 = F4(function(func, a, b, c)
{
	return _VirtualDom_thunk([func, a, b, c], function() {
		return A3(func, a, b, c);
	});
});

var _VirtualDom_lazy4 = F5(function(func, a, b, c, d)
{
	return _VirtualDom_thunk([func, a, b, c, d], function() {
		return A4(func, a, b, c, d);
	});
});

var _VirtualDom_lazy5 = F6(function(func, a, b, c, d, e)
{
	return _VirtualDom_thunk([func, a, b, c, d, e], function() {
		return A5(func, a, b, c, d, e);
	});
});

var _VirtualDom_lazy6 = F7(function(func, a, b, c, d, e, f)
{
	return _VirtualDom_thunk([func, a, b, c, d, e, f], function() {
		return A6(func, a, b, c, d, e, f);
	});
});

var _VirtualDom_lazy7 = F8(function(func, a, b, c, d, e, f, g)
{
	return _VirtualDom_thunk([func, a, b, c, d, e, f, g], function() {
		return A7(func, a, b, c, d, e, f, g);
	});
});

var _VirtualDom_lazy8 = F9(function(func, a, b, c, d, e, f, g, h)
{
	return _VirtualDom_thunk([func, a, b, c, d, e, f, g, h], function() {
		return A8(func, a, b, c, d, e, f, g, h);
	});
});



// FACTS


var _VirtualDom_on = F2(function(key, handler)
{
	return {
		$: 'a__1_EVENT',
		__key: key,
		__value: handler
	};
});
var _VirtualDom_style = F2(function(key, value)
{
	return {
		$: 'a__1_STYLE',
		__key: key,
		__value: value
	};
});
var _VirtualDom_property = F2(function(key, value)
{
	return {
		$: 'a__1_PROP',
		__key: key,
		__value: value
	};
});
var _VirtualDom_attribute = F2(function(key, value)
{
	return {
		$: 'a__1_ATTR',
		__key: key,
		__value: value
	};
});
var _VirtualDom_attributeNS = F3(function(namespace, key, value)
{
	return {
		$: 'a__1_ATTR_NS',
		__key: key,
		__value: { __namespace: namespace, __value: value }
	};
});



// XSS ATTACK VECTOR CHECKS


function _VirtualDom_noScript(tag)
{
	return tag == 'script' ? 'p' : tag;
}

function _VirtualDom_noOnOrFormAction(key)
{
	return /^(on|formAction$)/i.test(key) ? 'data-' + key : key;
}

function _VirtualDom_noInnerHtmlOrFormAction(key)
{
	return key == 'innerHTML' || key == 'formAction' ? 'data-' + key : key;
}

function _VirtualDom_noJavaScriptUri__PROD(value)
{
	return /^javascript:/i.test(value.replace(/\s/g,'')) ? '' : value;
}

function _VirtualDom_noJavaScriptUri__DEBUG(value)
{
	return /^javascript:/i.test(value.replace(/\s/g,''))
		? 'javascript:alert("This is an XSS vector. Please use ports or web components instead.")'
		: value;
}

function _VirtualDom_noJavaScriptOrHtmlUri__PROD(value)
{
	return /^\s*(javascript:|data:text\/html)/i.test(value) ? '' : value;
}

function _VirtualDom_noJavaScriptOrHtmlUri__DEBUG(value)
{
	return /^\s*(javascript:|data:text\/html)/i.test(value)
		? 'javascript:alert("This is an XSS vector. Please use ports or web components instead.")'
		: value;
}



// MAP FACTS


var _VirtualDom_mapAttribute = F2(function(func, attr)
{
	return (attr.$ === 'a__1_EVENT')
		? A2(_VirtualDom_on, attr.__key, _VirtualDom_mapHandler(func, attr.__value))
		: attr;
});

function _VirtualDom_mapHandler(func, handler)
{
	var tag = __VirtualDom_toHandlerInt(handler);

	// 0 = Normal
	// 1 = MayStopPropagation
	// 2 = MayPreventDefault
	// 3 = Custom

	return {
		$: handler.$,
		a:
			!tag
				? A2(__Json_map, func, handler.a)
				:
			A3(__Json_map2,
				tag < 3
					? _VirtualDom_mapEventTuple
					: _VirtualDom_mapEventRecord,
				__Json_succeed(func),
				handler.a
			)
	};
}

var _VirtualDom_mapEventTuple = F2(function(func, tuple)
{
	return __Utils_Tuple2(func(tuple.a), tuple.b);
});

var _VirtualDom_mapEventRecord = F2(function(func, record)
{
	return {
		__$message: func(record.__$message),
		__$stopPropagation: record.__$stopPropagation,
		__$preventDefault: record.__$preventDefault
	}
});



// ORGANIZE FACTS


function _VirtualDom_organizeFacts(factList)
{
	for (var facts = {}; factList.b; factList = factList.b) // WHILE_CONS
	{
		var entry = factList.a;

		var tag = entry.$;
		var key = entry.__key;
		var value = entry.__value;

		if (tag === 'a__1_PROP')
		{
			(key === 'className')
				? _VirtualDom_addClass(facts, key, __Json_unwrap(value))
				: facts[key] = __Json_unwrap(value);

			continue;
		}

		var subFacts = facts[tag] || (facts[tag] = {});
		(tag === 'a__1_ATTR' && key === 'class')
			? _VirtualDom_addClass(subFacts, key, value)
			: subFacts[key] = value;
	}

	return facts;
}

function _VirtualDom_addClass(object, key, newClass)
{
	var classes = object[key];
	object[key] = classes ? classes + ' ' + newClass : newClass;
}



// RENDER


function _VirtualDom_render(vNode, eventNode)
{
	var tag = vNode.$;

	if (tag === __2_THUNK)
	{
		return _VirtualDom_render(vNode.__node || (vNode.__node = vNode.__thunk()), eventNode);
	}

	if (tag === __2_TEXT)
	{
		return _VirtualDom_doc.createTextNode(vNode.__text);
	}

	if (tag === __2_TAGGER)
	{
		var subNode = vNode.__node;
		var tagger = vNode.__tagger;

		while (subNode.$ === __2_TAGGER)
		{
			typeof tagger !== 'object'
				? tagger = [tagger, subNode.__tagger]
				: tagger.push(subNode.__tagger);

			subNode = subNode.__node;
		}

		var subEventRoot = { __tagger: tagger, __parent: eventNode };
		var domNode = _VirtualDom_render(subNode, subEventRoot);
		domNode.elm_event_node_ref = subEventRoot;
		return domNode;
	}

	if (tag === __2_CUSTOM)
	{
		var domNode = vNode.__render(vNode.__model);
		_VirtualDom_applyFacts(domNode, eventNode, vNode.__facts);
		return domNode;
	}

	// at this point `tag` must be __2_NODE or __2_KEYED_NODE

	var domNode = vNode.__namespace
		? _VirtualDom_doc.createElementNS(vNode.__namespace, vNode.__tag)
		: _VirtualDom_doc.createElement(vNode.__tag);

	if (_VirtualDom_divertHrefToApp && vNode.__tag == 'a')
	{
		domNode.addEventListener('click', _VirtualDom_divertHrefToApp(domNode));
	}

	_VirtualDom_applyFacts(domNode, eventNode, vNode.__facts);

	for (var kids = vNode.__kids, i = 0; i < kids.length; i++)
	{
		_VirtualDom_appendChild(domNode, _VirtualDom_render(tag === __2_NODE ? kids[i] : kids[i].b, eventNode));
	}

	return domNode;
}



// APPLY FACTS


function _VirtualDom_applyFacts(domNode, eventNode, facts)
{
	for (var key in facts)
	{
		var value = facts[key];

		key === 'a__1_STYLE'
			? _VirtualDom_applyStyles(domNode, value)
			:
		key === 'a__1_EVENT'
			? _VirtualDom_applyEvents(domNode, eventNode, value)
			:
		key === 'a__1_ATTR'
			? _VirtualDom_applyAttrs(domNode, value)
			:
		key === 'a__1_ATTR_NS'
			? _VirtualDom_applyAttrsNS(domNode, value)
			:
		(key !== 'value' || key !== 'checked' || domNode[key] !== value) && (domNode[key] = value);
	}
}



// APPLY STYLES


function _VirtualDom_applyStyles(domNode, styles)
{
	var domNodeStyle = domNode.style;

	for (var key in styles)
	{
		domNodeStyle[key] = styles[key];
	}
}



// APPLY ATTRS


function _VirtualDom_applyAttrs(domNode, attrs)
{
	for (var key in attrs)
	{
		var value = attrs[key];
		value
			? domNode.setAttribute(key, value)
			: domNode.removeAttribute(key);
	}
}



// APPLY NAMESPACED ATTRS


function _VirtualDom_applyAttrsNS(domNode, nsAttrs)
{
	for (var key in nsAttrs)
	{
		var pair = nsAttrs[key];
		var namespace = pair.__namespace;
		var value = pair.__value;

		value
			? domNode.setAttributeNS(namespace, key, value)
			: domNode.removeAttributeNS(namespace, key);
	}
}



// APPLY EVENTS


function _VirtualDom_applyEvents(domNode, eventNode, events)
{
	var allCallbacks = domNode.elmFs || (domNode.elmFs = {});

	for (var key in events)
	{
		var newHandler = events[key];
		var oldCallback = allCallbacks[key];

		if (!newHandler)
		{
			domNode.removeEventListener(key, oldCallback);
			allCallbacks[key] = undefined;
			continue;
		}

		if (oldCallback)
		{
			var oldHandler = oldCallback.__handler;
			if (oldHandler.$ === newHandler.$)
			{
				oldCallback.__handler = newHandler;
				continue;
			}
			domNode.removeEventListener(key, oldCallback);
		}

		oldCallback = _VirtualDom_makeCallback(eventNode, newHandler);
		domNode.addEventListener(key, oldCallback,
			_VirtualDom_passiveSupported
			&& { passive: __VirtualDom_toHandlerInt(newHandler) < 2 }
		);
		allCallbacks[key] = oldCallback;
	}
}



// PASSIVE EVENTS


var _VirtualDom_passiveSupported;

try
{
	window.addEventListener('t', null, Object.defineProperty({}, 'passive', {
		get: function() { _VirtualDom_passiveSupported = true; }
	}));
}
catch(e) {}



// EVENT HANDLERS


function _VirtualDom_makeCallback(eventNode, initialHandler)
{
	function callback(event)
	{
		var handler = callback.__handler;
		var result = __Json_runHelp(handler.a, event);

		if (!__Result_isOk(result))
		{
			return;
		}

		var tag = __VirtualDom_toHandlerInt(handler);

		// 0 = Normal
		// 1 = MayStopPropagation
		// 2 = MayPreventDefault
		// 3 = Custom

		var value = result.a;
		var message = !tag ? value : tag < 3 ? value.a : value.__$message;
		var stopPropagation = tag == 1 ? value.b : tag == 3 && value.__$stopPropagation;
		var currentEventNode = (
			stopPropagation && event.stopPropagation(),
			(tag == 2 ? value.b : tag == 3 && value.__$preventDefault) && event.preventDefault(),
			eventNode
		);
		var tagger;
		var i;
		while (tagger = currentEventNode.__tagger)
		{
			if (typeof tagger == 'function')
			{
				message = tagger(message);
			}
			else
			{
				for (var i = tagger.length; i--; )
				{
					message = tagger[i](message);
				}
			}
			currentEventNode = currentEventNode.__parent;
		}
		currentEventNode(message, stopPropagation); // stopPropagation implies isSync
	}

	callback.__handler = initialHandler;

	return callback;
}

function _VirtualDom_equalEvents(x, y)
{
	return x.$ == y.$ && __Json_equality(x.a, y.a);
}



// DIFF


// TODO: Should we do patches like in iOS?
//
// type Patch
//   = At Int Patch
//   | Batch (List Patch)
//   | Change ...
//
// How could it not be better?
//
function _VirtualDom_diff(x, y)
{
	var patches = [];
	_VirtualDom_diffHelp(x, y, patches, 0);
	return patches;
}


function _VirtualDom_pushPatch(patches, type, index, data)
{
	var patch = {
		$: type,
		__index: index,
		__data: data,
		__domNode: undefined,
		__eventNode: undefined
	};
	patches.push(patch);
	return patch;
}


function _VirtualDom_diffHelp(x, y, patches, index)
{
	if (x === y)
	{
		return;
	}

	var xType = x.$;
	var yType = y.$;

	// Bail if you run into different types of nodes. Implies that the
	// structure has changed significantly and it's not worth a diff.
	if (xType !== yType)
	{
		if (xType === __2_NODE && yType === __2_KEYED_NODE)
		{
			y = _VirtualDom_dekey(y);
			yType = __2_NODE;
		}
		else
		{
			_VirtualDom_pushPatch(patches, __3_REDRAW, index, y);
			return;
		}
	}

	// Now we know that both nodes are the same $.
	switch (yType)
	{
		case __2_THUNK:
			var xRefs = x.__refs;
			var yRefs = y.__refs;
			var i = xRefs.length;
			var same = i === yRefs.length;
			while (same && i--)
			{
				same = xRefs[i] === yRefs[i];
			}
			if (same)
			{
				y.__node = x.__node;
				return;
			}
			y.__node = y.__thunk();
			var subPatches = [];
			_VirtualDom_diffHelp(x.__node, y.__node, subPatches, 0);
			subPatches.length > 0 && _VirtualDom_pushPatch(patches, __3_THUNK, index, subPatches);
			return;

		case __2_TAGGER:
			// gather nested taggers
			var xTaggers = x.__tagger;
			var yTaggers = y.__tagger;
			var nesting = false;

			var xSubNode = x.__node;
			while (xSubNode.$ === __2_TAGGER)
			{
				nesting = true;

				typeof xTaggers !== 'object'
					? xTaggers = [xTaggers, xSubNode.__tagger]
					: xTaggers.push(xSubNode.__tagger);

				xSubNode = xSubNode.__node;
			}

			var ySubNode = y.__node;
			while (ySubNode.$ === __2_TAGGER)
			{
				nesting = true;

				typeof yTaggers !== 'object'
					? yTaggers = [yTaggers, ySubNode.__tagger]
					: yTaggers.push(ySubNode.__tagger);

				ySubNode = ySubNode.__node;
			}

			// Just bail if different numbers of taggers. This implies the
			// structure of the virtual DOM has changed.
			if (nesting && xTaggers.length !== yTaggers.length)
			{
				_VirtualDom_pushPatch(patches, __3_REDRAW, index, y);
				return;
			}

			// check if taggers are "the same"
			if (nesting ? !_VirtualDom_pairwiseRefEqual(xTaggers, yTaggers) : xTaggers !== yTaggers)
			{
				_VirtualDom_pushPatch(patches, __3_TAGGER, index, yTaggers);
			}

			// diff everything below the taggers
			_VirtualDom_diffHelp(xSubNode, ySubNode, patches, index + 1);
			return;

		case __2_TEXT:
			if (x.__text !== y.__text)
			{
				_VirtualDom_pushPatch(patches, __3_TEXT, index, y.__text);
			}
			return;

		case __2_NODE:
			_VirtualDom_diffNodes(x, y, patches, index, _VirtualDom_diffKids);
			return;

		case __2_KEYED_NODE:
			_VirtualDom_diffNodes(x, y, patches, index, _VirtualDom_diffKeyedKids);
			return;

		case __2_CUSTOM:
			if (x.__render !== y.__render)
			{
				_VirtualDom_pushPatch(patches, __3_REDRAW, index, y);
				return;
			}

			var factsDiff = _VirtualDom_diffFacts(x.__facts, y.__facts);
			factsDiff && _VirtualDom_pushPatch(patches, __3_FACTS, index, factsDiff);

			var patch = y.__diff(x.__model, y.__model);
			patch && _VirtualDom_pushPatch(patches, __3_CUSTOM, index, patch);

			return;
	}
}

// assumes the incoming arrays are the same length
function _VirtualDom_pairwiseRefEqual(as, bs)
{
	for (var i = 0; i < as.length; i++)
	{
		if (as[i] !== bs[i])
		{
			return false;
		}
	}

	return true;
}

function _VirtualDom_diffNodes(x, y, patches, index, diffKids)
{
	// Bail if obvious indicators have changed. Implies more serious
	// structural changes such that it's not worth it to diff.
	if (x.__tag !== y.__tag || x.__namespace !== y.__namespace)
	{
		_VirtualDom_pushPatch(patches, __3_REDRAW, index, y);
		return;
	}

	var factsDiff = _VirtualDom_diffFacts(x.__facts, y.__facts);
	factsDiff && _VirtualDom_pushPatch(patches, __3_FACTS, index, factsDiff);

	diffKids(x, y, patches, index);
}



// DIFF FACTS


// TODO Instead of creating a new diff object, it's possible to just test if
// there *is* a diff. During the actual patch, do the diff again and make the
// modifications directly. This way, there's no new allocations. Worth it?
function _VirtualDom_diffFacts(x, y, category)
{
	var diff;

	// look for changes and removals
	for (var xKey in x)
	{
		if (xKey === 'a__1_STYLE' || xKey === 'a__1_EVENT' || xKey === 'a__1_ATTR' || xKey === 'a__1_ATTR_NS')
		{
			var subDiff = _VirtualDom_diffFacts(x[xKey], y[xKey] || {}, xKey);
			if (subDiff)
			{
				diff = diff || {};
				diff[xKey] = subDiff;
			}
			continue;
		}

		// remove if not in the new facts
		if (!(xKey in y))
		{
			diff = diff || {};
			diff[xKey] =
				!category
					? (typeof x[xKey] === 'string' ? '' : null)
					:
				(category === 'a__1_STYLE')
					? ''
					:
				(category === 'a__1_EVENT' || category === 'a__1_ATTR')
					? undefined
					:
				{ __namespace: x[xKey].__namespace, __value: undefined };

			continue;
		}

		var xValue = x[xKey];
		var yValue = y[xKey];

		// reference equal, so don't worry about it
		if (xValue === yValue && xKey !== 'value' && xKey !== 'checked'
			|| category === 'a__1_EVENT' && _VirtualDom_equalEvents(xValue, yValue))
		{
			continue;
		}

		diff = diff || {};
		diff[xKey] = yValue;
	}

	// add new stuff
	for (var yKey in y)
	{
		if (!(yKey in x))
		{
			diff = diff || {};
			diff[yKey] = y[yKey];
		}
	}

	return diff;
}



// DIFF KIDS


function _VirtualDom_diffKids(xParent, yParent, patches, index)
{
	var xKids = xParent.__kids;
	var yKids = yParent.__kids;

	var xLen = xKids.length;
	var yLen = yKids.length;

	// FIGURE OUT IF THERE ARE INSERTS OR REMOVALS

	if (xLen > yLen)
	{
		_VirtualDom_pushPatch(patches, __3_REMOVE_LAST, index, {
			__length: yLen,
			__diff: xLen - yLen
		});
	}
	else if (xLen < yLen)
	{
		_VirtualDom_pushPatch(patches, __3_APPEND, index, {
			__length: xLen,
			__kids: yKids
		});
	}

	// PAIRWISE DIFF EVERYTHING ELSE

	for (var minLen = xLen < yLen ? xLen : yLen, i = 0; i < minLen; i++)
	{
		var xKid = xKids[i];
		_VirtualDom_diffHelp(xKid, yKids[i], patches, ++index);
		index += xKid.__descendantsCount || 0;
	}
}



// KEYED DIFF


function _VirtualDom_diffKeyedKids(xParent, yParent, patches, rootIndex)
{
	var localPatches = [];

	var changes = {}; // Dict String Entry
	var inserts = []; // Array { index : Int, entry : Entry }
	// type Entry = { tag : String, vnode : VNode, index : Int, data : _ }

	var xKids = xParent.__kids;
	var yKids = yParent.__kids;
	var xLen = xKids.length;
	var yLen = yKids.length;
	var xIndex = 0;
	var yIndex = 0;

	var index = rootIndex;

	while (xIndex < xLen && yIndex < yLen)
	{
		var x = xKids[xIndex];
		var y = yKids[yIndex];

		var xKey = x.a;
		var yKey = y.a;
		var xNode = x.b;
		var yNode = y.b;

		// check if keys match

		if (xKey === yKey)
		{
			index++;
			_VirtualDom_diffHelp(xNode, yNode, localPatches, index);
			index += xNode.__descendantsCount || 0;

			xIndex++;
			yIndex++;
			continue;
		}

		// look ahead 1 to detect insertions and removals.

		var xNext = xKids[xIndex + 1];
		var yNext = yKids[yIndex + 1];

		if (xNext)
		{
			var xNextKey = xNext.a;
			var xNextNode = xNext.b;
			var oldMatch = yKey === xNextKey;
		}

		if (yNext)
		{
			var yNextKey = yNext.a;
			var yNextNode = yNext.b;
			var newMatch = xKey === yNextKey;
		}


		// swap x and y
		if (newMatch && oldMatch)
		{
			index++;
			_VirtualDom_diffHelp(xNode, yNextNode, localPatches, index);
			_VirtualDom_insertNode(changes, localPatches, xKey, yNode, yIndex, inserts);
			index += xNode.__descendantsCount || 0;

			index++;
			_VirtualDom_removeNode(changes, localPatches, xKey, xNextNode, index);
			index += xNextNode.__descendantsCount || 0;

			xIndex += 2;
			yIndex += 2;
			continue;
		}

		// insert y
		if (newMatch)
		{
			index++;
			_VirtualDom_insertNode(changes, localPatches, yKey, yNode, yIndex, inserts);
			_VirtualDom_diffHelp(xNode, yNextNode, localPatches, index);
			index += xNode.__descendantsCount || 0;

			xIndex += 1;
			yIndex += 2;
			continue;
		}

		// remove x
		if (oldMatch)
		{
			index++;
			_VirtualDom_removeNode(changes, localPatches, xKey, xNode, index);
			index += xNode.__descendantsCount || 0;

			index++;
			_VirtualDom_diffHelp(xNextNode, yNode, localPatches, index);
			index += xNextNode.__descendantsCount || 0;

			xIndex += 2;
			yIndex += 1;
			continue;
		}

		// remove x, insert y
		if (xNext && xNextKey === yNextKey)
		{
			index++;
			_VirtualDom_removeNode(changes, localPatches, xKey, xNode, index);
			_VirtualDom_insertNode(changes, localPatches, yKey, yNode, yIndex, inserts);
			index += xNode.__descendantsCount || 0;

			index++;
			_VirtualDom_diffHelp(xNextNode, yNextNode, localPatches, index);
			index += xNextNode.__descendantsCount || 0;

			xIndex += 2;
			yIndex += 2;
			continue;
		}

		break;
	}

	// eat up any remaining nodes with removeNode and insertNode

	while (xIndex < xLen)
	{
		index++;
		var x = xKids[xIndex];
		var xNode = x.b;
		_VirtualDom_removeNode(changes, localPatches, x.a, xNode, index);
		index += xNode.__descendantsCount || 0;
		xIndex++;
	}

	while (yIndex < yLen)
	{
		var endInserts = endInserts || [];
		var y = yKids[yIndex];
		_VirtualDom_insertNode(changes, localPatches, y.a, y.b, undefined, endInserts);
		yIndex++;
	}

	if (localPatches.length > 0 || inserts.length > 0 || endInserts)
	{
		_VirtualDom_pushPatch(patches, __3_REORDER, rootIndex, {
			__patches: localPatches,
			__inserts: inserts,
			__endInserts: endInserts
		});
	}
}



// CHANGES FROM KEYED DIFF


var _VirtualDom_POSTFIX = '_elmW6BL';


function _VirtualDom_insertNode(changes, localPatches, key, vnode, yIndex, inserts)
{
	var entry = changes[key];

	// never seen this key before
	if (!entry)
	{
		entry = {
			__tag: __5_INSERT,
			__vnode: vnode,
			__index: yIndex,
			__data: undefined
		};

		inserts.push({ __index: yIndex, __entry: entry });
		changes[key] = entry;

		return;
	}

	// this key was removed earlier, a match!
	if (entry.__tag === __5_REMOVE)
	{
		inserts.push({ __index: yIndex, __entry: entry });

		entry.__tag = __5_MOVE;
		var subPatches = [];
		_VirtualDom_diffHelp(entry.__vnode, vnode, subPatches, entry.__index);
		entry.__index = yIndex;
		entry.__data.__data = {
			__patches: subPatches,
			__entry: entry
		};

		return;
	}

	// this key has already been inserted or moved, a duplicate!
	_VirtualDom_insertNode(changes, localPatches, key + _VirtualDom_POSTFIX, vnode, yIndex, inserts);
}


function _VirtualDom_removeNode(changes, localPatches, key, vnode, index)
{
	var entry = changes[key];

	// never seen this key before
	if (!entry)
	{
		var patch = _VirtualDom_pushPatch(localPatches, __3_REMOVE, index, undefined);

		changes[key] = {
			__tag: __5_REMOVE,
			__vnode: vnode,
			__index: index,
			__data: patch
		};

		return;
	}

	// this key was inserted earlier, a match!
	if (entry.__tag === __5_INSERT)
	{
		entry.__tag = __5_MOVE;
		var subPatches = [];
		_VirtualDom_diffHelp(vnode, entry.__vnode, subPatches, index);

		_VirtualDom_pushPatch(localPatches, __3_REMOVE, index, {
			__patches: subPatches,
			__entry: entry
		});

		return;
	}

	// this key has already been removed or moved, a duplicate!
	_VirtualDom_removeNode(changes, localPatches, key + _VirtualDom_POSTFIX, vnode, index);
}



// ADD DOM NODES
//
// Each DOM node has an "index" assigned in order of traversal. It is important
// to minimize our crawl over the actual DOM, so these indexes (along with the
// descendantsCount of virtual nodes) let us skip touching entire subtrees of
// the DOM if we know there are no patches there.


function _VirtualDom_addDomNodes(domNode, vNode, patches, eventNode)
{
	_VirtualDom_addDomNodesHelp(domNode, vNode, patches, 0, 0, vNode.__descendantsCount, eventNode);
}


// assumes `patches` is non-empty and indexes increase monotonically.
function _VirtualDom_addDomNodesHelp(domNode, vNode, patches, i, low, high, eventNode)
{
	var patch = patches[i];
	var index = patch.__index;

	while (index === low)
	{
		var patchType = patch.$;

		if (patchType === __3_THUNK)
		{
			_VirtualDom_addDomNodes(domNode, vNode.__node, patch.__data, eventNode);
		}
		else if (patchType === __3_REORDER)
		{
			patch.__domNode = domNode;
			patch.__eventNode = eventNode;

			var subPatches = patch.__data.__patches;
			if (subPatches.length > 0)
			{
				_VirtualDom_addDomNodesHelp(domNode, vNode, subPatches, 0, low, high, eventNode);
			}
		}
		else if (patchType === __3_REMOVE)
		{
			patch.__domNode = domNode;
			patch.__eventNode = eventNode;

			var data = patch.__data;
			if (data)
			{
				data.__entry.__data = domNode;
				var subPatches = data.__patches;
				if (subPatches.length > 0)
				{
					_VirtualDom_addDomNodesHelp(domNode, vNode, subPatches, 0, low, high, eventNode);
				}
			}
		}
		else
		{
			patch.__domNode = domNode;
			patch.__eventNode = eventNode;
		}

		i++;

		if (!(patch = patches[i]) || (index = patch.__index) > high)
		{
			return i;
		}
	}

	var tag = vNode.$;

	if (tag === __2_TAGGER)
	{
		var subNode = vNode.__node;

		while (subNode.$ === __2_TAGGER)
		{
			subNode = subNode.__node;
		}

		return _VirtualDom_addDomNodesHelp(domNode, subNode, patches, i, low + 1, high, domNode.elm_event_node_ref);
	}

	// tag must be __2_NODE or __2_KEYED_NODE at this point

	var vKids = vNode.__kids;
	var childNodes = domNode.childNodes;
	for (var j = 0; j < vKids.length; j++)
	{
		low++;
		var vKid = tag === __2_NODE ? vKids[j] : vKids[j].b;
		var nextLow = low + (vKid.__descendantsCount || 0);
		if (low <= index && index <= nextLow)
		{
			i = _VirtualDom_addDomNodesHelp(childNodes[j], vKid, patches, i, low, nextLow, eventNode);
			if (!(patch = patches[i]) || (index = patch.__index) > high)
			{
				return i;
			}
		}
		low = nextLow;
	}
	return i;
}



// APPLY PATCHES


function _VirtualDom_applyPatches(rootDomNode, oldVirtualNode, patches, eventNode)
{
	if (patches.length === 0)
	{
		return rootDomNode;
	}

	_VirtualDom_addDomNodes(rootDomNode, oldVirtualNode, patches, eventNode);
	return _VirtualDom_applyPatchesHelp(rootDomNode, patches);
}

function _VirtualDom_applyPatchesHelp(rootDomNode, patches)
{
	for (var i = 0; i < patches.length; i++)
	{
		var patch = patches[i];
		var localDomNode = patch.__domNode
		var newNode = _VirtualDom_applyPatch(localDomNode, patch);
		if (localDomNode === rootDomNode)
		{
			rootDomNode = newNode;
		}
	}
	return rootDomNode;
}

function _VirtualDom_applyPatch(domNode, patch)
{
	switch (patch.$)
	{
		case __3_REDRAW:
			return _VirtualDom_applyPatchRedraw(domNode, patch.__data, patch.__eventNode);

		case __3_FACTS:
			_VirtualDom_applyFacts(domNode, patch.__eventNode, patch.__data);
			return domNode;

		case __3_TEXT:
			domNode.replaceData(0, domNode.length, patch.__data);
			return domNode;

		case __3_THUNK:
			return _VirtualDom_applyPatchesHelp(domNode, patch.__data);

		case __3_TAGGER:
			if (domNode.elm_event_node_ref)
			{
				domNode.elm_event_node_ref.__tagger = patch.__data;
			}
			else
			{
				domNode.elm_event_node_ref = { __tagger: patch.__data, __parent: patch.__eventNode };
			}
			return domNode;

		case __3_REMOVE_LAST:
			var data = patch.__data;
			for (var i = 0; i < data.__diff; i++)
			{
				domNode.removeChild(domNode.childNodes[data.__length]);
			}
			return domNode;

		case __3_APPEND:
			var data = patch.__data;
			var kids = data.__kids;
			var i = data.__length;
			var theEnd = domNode.childNodes[i];
			for (; i < kids.length; i++)
			{
				domNode.insertBefore(_VirtualDom_render(kids[i], patch.__eventNode), theEnd);
			}
			return domNode;

		case __3_REMOVE:
			var data = patch.__data;
			if (!data)
			{
				domNode.parentNode.removeChild(domNode);
				return domNode;
			}
			var entry = data.__entry;
			if (typeof entry.__index !== 'undefined')
			{
				domNode.parentNode.removeChild(domNode);
			}
			entry.__data = _VirtualDom_applyPatchesHelp(domNode, data.__patches);
			return domNode;

		case __3_REORDER:
			return _VirtualDom_applyPatchReorder(domNode, patch);

		case __3_CUSTOM:
			return patch.__data(domNode);

		default:
			__Debug_crash(10); // 'Ran into an unknown patch!'
	}
}


function _VirtualDom_applyPatchRedraw(domNode, vNode, eventNode)
{
	var parentNode = domNode.parentNode;
	var newNode = _VirtualDom_render(vNode, eventNode);

	if (!newNode.elm_event_node_ref)
	{
		newNode.elm_event_node_ref = domNode.elm_event_node_ref;
	}

	if (parentNode && newNode !== domNode)
	{
		parentNode.replaceChild(newNode, domNode);
	}
	return newNode;
}


function _VirtualDom_applyPatchReorder(domNode, patch)
{
	var data = patch.__data;

	// remove end inserts
	var frag = _VirtualDom_applyPatchReorderEndInsertsHelp(data.__endInserts, patch);

	// removals
	domNode = _VirtualDom_applyPatchesHelp(domNode, data.__patches);

	// inserts
	var inserts = data.__inserts;
	for (var i = 0; i < inserts.length; i++)
	{
		var insert = inserts[i];
		var entry = insert.__entry;
		var node = entry.__tag === __5_MOVE
			? entry.__data
			: _VirtualDom_render(entry.__vnode, patch.__eventNode);
		domNode.insertBefore(node, domNode.childNodes[insert.__index]);
	}

	// add end inserts
	if (frag)
	{
		_VirtualDom_appendChild(domNode, frag);
	}

	return domNode;
}


function _VirtualDom_applyPatchReorderEndInsertsHelp(endInserts, patch)
{
	if (!endInserts)
	{
		return;
	}

	var frag = _VirtualDom_doc.createDocumentFragment();
	for (var i = 0; i < endInserts.length; i++)
	{
		var insert = endInserts[i];
		var entry = insert.__entry;
		_VirtualDom_appendChild(frag, entry.__tag === __5_MOVE
			? entry.__data
			: _VirtualDom_render(entry.__vnode, patch.__eventNode)
		);
	}
	return frag;
}


function _VirtualDom_virtualize(node)
{
	// TEXT NODES

	if (node.nodeType === 3)
	{
		return _VirtualDom_text(node.textContent);
	}


	// WEIRD NODES

	if (node.nodeType !== 1)
	{
		return _VirtualDom_text('');
	}


	// ELEMENT NODES

	var attrList = __List_Nil;
	var attrs = node.attributes;
	for (var i = attrs.length; i--; )
	{
		var attr = attrs[i];
		var name = attr.name;
		var value = attr.value;
		attrList = __List_Cons( A2(_VirtualDom_attribute, name, value), attrList );
	}

	var tag = node.tagName.toLowerCase();
	var kidList = __List_Nil;
	var kids = node.childNodes;

	for (var i = kids.length; i--; )
	{
		kidList = __List_Cons(_VirtualDom_virtualize(kids[i]), kidList);
	}
	return A3(_VirtualDom_node, tag, attrList, kidList);
}

function _VirtualDom_dekey(keyedNode)
{
	var keyedKids = keyedNode.__kids;
	var len = keyedKids.length;
	var kids = new Array(len);
	for (var i = 0; i < len; i++)
	{
		kids[i] = keyedKids[i].b;
	}

	return {
		$: __2_NODE,
		__tag: keyedNode.__tag,
		__facts: keyedNode.__facts,
		__kids: kids,
		__namespace: keyedNode.__namespace,
		__descendantsCount: keyedNode.__descendantsCount
	};
}