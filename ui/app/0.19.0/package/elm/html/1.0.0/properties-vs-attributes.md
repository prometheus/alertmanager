# Properties vs. Attributes

When you are working with HTML, you can have DOM nodes like `<div></div>`. And you can add *attributes* to those DOM nodes, like `<div class="user-info"></div>`.

When you are creating DOM nodes in JavaScript, there are two ways to add attributes like this:

  1. **Attributes** &mdash; You can use the `setAttribute` function. So adding a class attribute would look like this: `domNode.setAttribute('class', 'user-info')`.

  2. **Properties** &mdash; JavaScript often exposes an alternate way to set these attributes. Another way to add a class attribute would be like this: `domNode.className = 'user-info'`

Notice that the attribute is called `class` but the property is called `className`. This is because `class` is a reserved word in JavaScript. Point is, **attributes and properties do not always match up.**

It is actually a bit crazier than that though. **Sometimes an attribute exists, but there is no corresponding property.** For example, as of this writing the `webkit-playsinline` can be added with `setAttribute`, but there is no corresponding property. And with SVG, you cannot use properties at all, you must to use `setAttributeNS` for everything.

With all the corner cases here, it makes sense to have access to both approaches.