# HTML

Quickly render HTML in Elm.


## Examples

The HTML part of an Elm program looks something like this:

```elm
import Html exposing (Html, button, div, text)
import Html.Events exposing (onClick)

type Msg = Increment | Decrement

view : Int -> Html Msg
view count =
  div []
    [ button [ onClick Decrement ] [ text "-" ]
    , div [] [ text (String.fromInt count) ]
    , button [ onClick Increment ] [ text "+" ]
    ]
```

If you call `view 42` you get something like this:

```html
<div>
  <button>-</button>
  <div>42</div>
  <button>+</button>
</div>
```

This snippet comes from a complete example. You can play with it online [here](https://elm-lang.org/examples/buttons) and read how it works [here](https://guide.elm-lang.org/architecture/user_input/buttons.html).

You can play with a bunch of other examples [here](https://elm-lang.org/examples).


## Learn More

**Definitely read through [guide.elm-lang.org](https://guide.elm-lang.org/) to understand how this all works!** The section on [The Elm Architecture](https://guide.elm-lang.org/architecture/index.html) is particularly helpful.


## Implementation

This library is backed by [elm/virtual-dom](https://package.elm-lang.org/packages/elm/virtual-dom/latest/) which handles the dirty details of rendering DOM nodes quickly. You can read some blog posts about it here:

  - [Blazing Fast HTML, Round Two](https://elm-lang.org/blog/blazing-fast-html-round-two)
  - [Blazing Fast HTML](https://elm-lang.org/blog/blazing-fast-html)
