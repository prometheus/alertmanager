# How do I manage URL from a `Browser.element`?

Many companies introduce Elm gradually. They use `Browser.element` to embed Elm in a larger codebase as a low-risk way to see if Elm is helpful. If so, great, do more! If not, just revert, no big deal.

But at some companies the element has grown to manage _almost_ the whole page. Everything except the header and footer, which are produced by the server. And at that time, you may want Elm to start managing URL changes, showing different things in different cases. Well, `Browser.application` lets you do that in Elm, but maybe you have a bunch of legacy code that still needs the header and footer to be created on the server, so `Browser.element` is the only option.

What do you do?


## Managing the URL from `Browser.element`

You would initialize your element like this:

```javascript
// Initialize your Elm program
var app = Elm.Main.init({
    flags: location.href,
    node: document.getElementById('elm-main')
});

// Inform app of browser navigation (the BACK and FORWARD buttons)
document.addEventListener('popstate', function () {
    app.ports.onUrlChange.send(location.href);
});

// Change the URL upon request, inform app of the change.
app.ports.pushUrl.subscribe(function(url) {
    history.pushState({}, '', url);
    app.ports.onUrlChange.send(location.href);
});
```

Now the important thing is that you can handle other things in these two event listeners. Maybe your header is sensitive to the URL as well? This is where you manage
anything like that.

From there, your Elm code would look something like this:

```elm
import Browser
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import Json.Decode as D
import Url
import Url.Parser as Url


main : Program String Model Msg
main =
  Browser.element
    { init = init
    , view = view
    , update = update
    , subscriptions = subscriptions
    }


type Msg = UrlChanged (Maybe Route) | ...


-- INIT

init : String -> ( Model, Cmd Msg )
init locationHref =
  ...


-- SUBSCRIPTION

subscriptions : Model -> Sub Msg
subscriptions model =
  onUrlChange UrlChanged


-- NAVIGATION

port onUrlChange : (String -> msg) -> Sub msg

port pushUrl : String -> Cmd msg

link msg -> List (Attribute msg) -> List (Html msg) -> Html msg
link href attrs children =
  a (preventDefaultOn "click" (D.succeed (href, True)) :: attrs) children

locationHrefToRoute : String -> Maybe Route
locationHrefToRoute locationHref =
  case Url.fromString locationHref of
    Nothing -> Nothing
    Just url -> Url.parse myParser url

-- myParser : Url.Parser (Route -> Route) Route
```

So in contrast with `Browser.application`, you have to manage the URL yourself in JavaScript. What is up with that?!


## Justification

The justification is that (1) this will lead to more reliable programs overall and (2) other designs do not save significant amounts of code. We will explore both in order.


### Reliability

There are some Elm users that have many different technologies embedded in the same document. So imagine we have a header in React, charts in Elm, and a data entry interface in Angular.

For URL management to work here, all three of these things need to agree on what page they are on. So the most reliable design is to have one `popstate` listener on the very outside. It would tell React, Elm, and Angular what to do. This gives you a guarantee that they are all in agreement about the current page. Similarly, they would all send messages out requesting a `pushState` such that everyone is informed of any changes.

If each project was reacting to the URL internally, synchronization bugs would inevitably arise. Maybe it was a static page, but it upgraded to have the URL change. You added that to your Elm, but what about the Angular and React elements. What happens to them? Probably people forget and it is just a confusing bug. So having one `popstate` makes it obvious that there is a decision to make here. And what happens when React starts producing URLs that Angular and Elm have never heard of? Do those elements show some sort of 404 page?

> **Note:** If you wanted you could send the `location.href` into a `Platform.worker` to do the URL parsing in Elm. Once you have nice data, you could send it out a port for all the different elements on your page.


### Lines of Code

So the decision is primarily motivated by the fact that **URL management should happen at the highest possible level for reliability**, but what if Elm is the only thing on screen? How many lines extra are those people paying?

Well, the JavaScript code would be something like this:

```javascript
var app = Elm.Main.init({
    flags: ...
});
```

And in Elm:

```elm
import Browser
import Browser.Navigation as Nav
import Url
import Url.Parser as Url


main : Program Flags Model Msg
main =
  Browser.application
    { init = init
    , view = view
    , update = update
    , subscriptions = subscriptions
    , onUrlChange = UrlChanged
    , onUrlRequest = LinkClicked
    }


type Msg = UrlChanged (Maybe Route) | ...


-- INIT

init : Flags -> Url.Url -> Nav.Key -> ( Model, Cmd Msg )
init flags url key =
  ...


-- SUBSCRIPTION

subscriptions : Model -> Sub Msg
subscriptions model =
  Sub.none


-- NAVIGATION

urlToRoute : Url.Url -> Maybe Route
urlToRoute url =
  Url.parse myParser url

-- myParser : Url.Parser (Route -> Route) Route
```

So the main differences are:

1. You can delete the ports in JavaScript (seven lines)
2. `port onUrlChanged` becomes `onUrlChanged` in `main` (zero lines)
3. `locationHrefToRoute` becomes `urlToRoute` (three lines)
4. `link` becomes `onUrlRequest` and handling code in `update` (depends)

So we are talking about maybe twenty lines of code that go away in the `application` version? And each line has a very clear purpose, allowing you to customize and synchronize based on your exact application. Maybe you only want the hash because you support certain IE browsers? Change the `popstate` listener to `hashchange`. Maybe you only want the last two segments of the URL because the rest is managed in React? Change `locationHrefToRoute` to be `whateverToRoute` based on what you need. Etc.


### Summary

It seems appealing to &ldquo;just do the same thing&rdquo; in `Browser.element` as in `Browser.application`, but you quickly run into corner cases when you consider the broad range of people and companies using Elm. When Elm and React are on the same page, who owns the URL? When `history.pushState` is called in React, how does Elm hear about it? When `pushUrl` is called in Elm, how does React hear about it? It does not appear that there actually _is_ a simpler or shorter way for `Browser.element` to handle these questions. Special hooks on the JS side? And what about the folks using `Browser.element` who are not messing with the URL?

By keeping it super simple (1) your attention is drawn to the fact that there are actually tricky situations to consider, (2) you have the flexibility to handle any situation that comes up, and (3) folks who are _not_ managing the URL from embedded Elm (the vast majority!) get a `Browser.element` with no extra details.

The current design seems to balance all these competing concerns in a nice way, even if it may seem like one _particular_ scenario could be a bit better.
