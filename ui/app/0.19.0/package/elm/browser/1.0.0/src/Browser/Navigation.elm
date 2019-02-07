module Browser.Navigation exposing
  ( Key
  , pushUrl
  , replaceUrl
  , back
  , forward
  , load
  , reload
  , reloadAndSkipCache
  )


{-| This module helps you manage the browser’s URL yourself. This is the
crucial trick when using [`Browser.application`](Browser#application).

The most important function is [`pushUrl`](#pushUrl) which changes the
address bar *without* starting a page load.


## What is a page load?

  1. Request a new HTML document. The page goes blank.
  2. As the HTML loads, request any `<script>` or `<link>` resources.
  3. A `<script>` may mutate the document, so these tags block rendering.
  4. When *all* of the assets are loaded, actually render the page.

That means the page will go blank for at least two round-trips to the servers!
You may have 90% of the data you need and be blocked on a font that is taking
a long time. Still blank!


## How does `pushUrl` help?

The `pushUrl` function changes the URL, but lets you keep the current HTML.
This means the page *never* goes blank. Instead of making two round-trips to
the server, you load whatever assets you want from within Elm. Maybe you do
not need any round-trips! Meanwhile, you retain full control over the UI, so
you can show a loading bar, show information as it loads, etc. Whatever you
want!


# Navigate within Page
@docs Key, pushUrl, replaceUrl, back, forward

# Navigate to other Pages
@docs load, reload, reloadAndSkipCache

-}


import Elm.Kernel.Browser
import Task exposing (Task)



-- WITHIN PAGE


{-| A navigation `Key` is needed to create navigation commands that change the
URL. That includes [`pushUrl`](#pushUrl), [`replaceUrl`](#replaceUrl),
[`back`](#back), and [`forward`](#forward).

You only get access to a `Key` when you create your program with
[`Browser.application`](Browser#application), guaranteeing that your program is
equipped to detect these URL changes. If `Key` values were available in other
kinds of programs, unsuspecting programmers would be sure to run into some
[annoying bugs][bugs] and learn a bunch of techniques the hard way!

[bugs]: https://github.com/elm/browser/blob/1.0.0/notes/navigation-in-elements.md
-}
type Key = Key


{-| Change the URL, but do not trigger a page load.

This will add a new entry to the browser history.

Check out the [`elm/url`][url] package for help building URLs. The
[`Url.absolute`][abs] and [`Url.relative`][rel] functions can be particularly
handy!

[url]: /packages/elm/url/latest
[abs]: /packages/elm/url/latest/Url#absolute
[rel]: /packages/elm/url/latest/Url#relative

**Note:** If the user has gone `back` a few pages, there will be &ldquo;future
pages&rdquo; that the user can go `forward` to. Adding a new URL in that
scenario will clear out any future pages. It is like going back in time and
making a different choice.
-}
pushUrl : Key -> String -> Cmd msg
pushUrl =
  Elm.Kernel.Browser.pushUrl


{-| Change the URL, but do not trigger a page load.

This *will not* add a new entry to the browser history.

This can be useful if you have search box and you want the `?search=hats` in
the URL to match without adding a history entry for every single key stroke.
Imagine how annoying it would be to click `back` thirty times and still be on
the same page!
-}
replaceUrl : Key -> String -> Cmd msg
replaceUrl =
  Elm.Kernel.Browser.replaceUrl


{-| Go back some number of pages. So `back 1` goes back one page, and `back 2`
goes back two pages.

**Note:** You only manage the browser history that *you* created. Think of this
library as letting you have access to a small part of the overall history. So
if you go back farther than the history you own, you will just go back to some
other website!
-}
back : Key -> Int -> Cmd msg
back key n =
  Elm.Kernel.Browser.go key -n


{-| Go forward some number of pages. So `forward 1` goes forward one page, and
`forward 2` goes forward two pages. If there are no more pages in the future,
this will do nothing.

**Note:** You only manage the browser history that *you* created. Think of this
library as letting you have access to a small part of the overall history. So
if you go forward farther than the history you own, the user will end up on
whatever website they visited next!
-}
forward : Key -> Int -> Cmd msg
forward =
  Elm.Kernel.Browser.go



-- EXTERNAL PAGES


{-| Leave the current page and load the given URL. **This always results in a
page load**, even if the provided URL is the same as the current one.

    gotoElmWebsite : Cmd msg
    gotoElmWebsite =
      load "https://elm-lang.org"

Check out the [`elm/url`][url] package for help building URLs. The
[`Url.absolute`][abs] and [`Url.relative`][rel] functions can be particularly
handy!

[url]: /packages/elm/url/latest
[abs]: /packages/elm/url/latest/Url#absolute
[rel]: /packages/elm/url/latest/Url#relative

-}
load : String -> Cmd msg
load =
  Elm.Kernel.Browser.load


{-| Reload the current page. **This always results in a page load!**
This may grab resources from the browser cache, so use
[`reloadAndSkipCache`](#reloadAndSkipCache)
if you want to be sure that you are not loading any cached resources.
-}
reload : Cmd msg
reload =
  Elm.Kernel.Browser.reload False


{-| Reload the current page without using the browser cache. **This always
results in a page load!** It is more common to want [`reload`](#reload).
-}
reloadAndSkipCache : Cmd msg
reloadAndSkipCache =
  Elm.Kernel.Browser.reload True
