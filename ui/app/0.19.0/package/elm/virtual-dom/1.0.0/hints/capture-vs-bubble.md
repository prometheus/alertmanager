Events flow through the DOM in two phases:

<span style="font-family:'Courier New',Courier;">
```
    Capture     Bubble
       ↓          ↑
┌──────↓──────────↑──────┐
│      ↓   Body   ↑      │
│ ┌────↓──────────↑────┐ │
│ │    ↓    Div   ↑    │ │
│ │ ┌──↓──────────↑──┐ │ │
│ │ │     Button     │ │ │
│ │ └────────────────┘ │ │
│ └────────────────────┘ │
└────────────────────────┘
```
</span>

In the old times, Microsoft only had `Bubble` and Netscape only had `Capture`. It was not ideal.

Today browsers support both, and **`Bubble` is the default** for
`addEventListener` in JavaScript and for all the functions in
[`Html.Events`](http://package.elm-lang.org/packages/elm-lang/html/latest/Html-Events).

It is extremely difficult to get `Capture` to behave predictably, especially if you are doing any `stopPropagation` tricks. It does not seem to be a great idea, and it seems too late to remove it.
