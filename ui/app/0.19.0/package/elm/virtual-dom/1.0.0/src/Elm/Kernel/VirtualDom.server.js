/*

*/


// TEXT

function _VirtualDom_text(string)
{
	return string.replace(_VirtualDom_badChars, _VirtualDom_replaceBadChars);
}

var _VirtualDom_badChars = /[&<>"']/g;

var _VirtualDom_table = {
	'&': "&amp;",
	'<': "&lt;",
	'>': "&gt;",
	'"': "&quot;",
	"'": "&#039;"
};

function _VirtualDom_replaceBadChars(m)
{
	return _VirtualDom_table[m];
}


// BODY

function _VirtualDom_body(kidList)
{
	var output = '';

	for (; kidList.b; kidList = kidList.b) // WHILE_CONS
	{
		output += kidList.a;
	}

	return output;
}


// NODES

var _VirtualDom_nodeNS = F2(function(namespace, tag)
{
	return F2(function(factList, kidList)
	{
		var output = _VirtualDom_openTag(tag, factList);

		for (; kidList.b; kidList = kidList.b) // WHILE_CONS
		{
			output += kidList.a;
		}

		return output + '</' + tag + '>';
	});
});

var _VirtualDom_node = _VirtualDom_nodeNS(undefined);


// KEYED NODES

var _VirtualDom_keyedNodeNS = F2(function(namespace, tag)
{
	return F2(function(factList, kidList)
	{
		var output = _VirtualDom_openTag(tag, factList);

		for (; kidList.b; kidList = kidList.b) // WHILE_CONS
		{
			output += kidList.a.b;
		}

		return output + '</' + tag + '>';
	});
});

var _VirtualDom_keyedNode = _VirtualDom_keyedNodeNS(undefined);


// CUSTOM

var _VirtualDom_custom = F3(function(factList, model, impl)
{
	return _VirtualDom_openTag('div', factList) + '</div>';
});


// MAP

var _VirtualDom_map = F2(function(tagger, node)
{
	return node;
});


// LAZY

var _VirtualDom_lazy = F2(function(fn, a)
{
	return fn(a);
});

var _VirtualDom_lazy2 = F3(function(fn, a, b)
{
	return A2(fn, a, b);
});

var _VirtualDom_lazy3 = F4(function(fn, a, b, c)
{
	return A3(fn, a, b, c);
});

var _VirtualDom_lazy4 = F5(function(fn, a, b, c, d)
{
	return A4(fn, a, b, c, d);
});

var _VirtualDom_lazy5 = F6(function(fn, a, b, c, d, e)
{
	return A5(fn, a, b, c, d, e);
});

var _VirtualDom_lazy6 = F7(function(fn, a, b, c, d, e, f)
{
	return A6(fn, a, b, c, d, e, f);
});

var _VirtualDom_lazy7 = F8(function(fn, a, b, c, d, e, f, g)
{
	return A7(fn, a, b, c, d, e, f, g);
});

var _VirtualDom_lazy8 = F9(function(fn, a, b, c, d, e, f, g, h)
{
	return A8(fn, a, b, c, d, e, f, g, h);
});


// FACTS

function _VirtualDom_openTag(tag, facts)
{
	var output = '<' + tag;
	var styles = ' style="';
	for (; facts.b; facts = facts.b) // WHILE_CONS
	{
		var fact = facts.a;
		if (fact instanceof String)
		{
			styles += fact;
		}
		else
		{
			output += fact;
		}
	}

	return (styles.length > 8
		? output + styles + '">'
		: output + '>'
	);
}


// FACTS

var _VirtualDom_style = F2(function(key, value)
{
	return new String(key + ':' + value + ';');
});

var _VirtualDom_attribute = F2(function(key, value)
{
	return ' ' + key + '="' + value + '"';
});

var _VirtualDom_attributeNS = F3(function(namespace, key, value)
{
	return '';
});

var _VirtualDom_on = F2(function(name, decoder)
{
	return '';
});

var _VirtualDom_property = F2(function(key, value)
{
	return ' ' + (propertyToAttribute[key] || key) + '="' + value + '"';
});

var propertyToAttribute = {
	'className': 'class',
	'htmlFor': 'for',
	'httpEquiv': 'http-equiv',
	'acceptCharset': 'accept-charset'
};


// MAP ATTRIBUTE

var _VirtualDom_mapAttribute = F2(function(func, attr)
{
	return attr;
});
