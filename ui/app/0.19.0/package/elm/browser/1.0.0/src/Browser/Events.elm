effect module Browser.Events where { subscription = MySub } exposing
  ( onAnimationFrame, onAnimationFrameDelta
  , onKeyPress, onKeyDown, onKeyUp
  , onClick, onMouseMove, onMouseDown, onMouseUp
  , onResize, onVisibilityChange, Visibility(..)
  )


{-| In JavaScript, information about the root of an HTML document is held in
the `document` and `window` objects. This module lets you create event
listeners on those objects for the following topics: [animation](#animation),
[keyboard](#keyboard), [mouse](#mouse), and [window](#window).

If there is something else you need, use [ports][] to do it in JavaScript!

[ports]: https://guide.elm-lang.org/interop/ports.html

# Animation
@docs onAnimationFrame, onAnimationFrameDelta

# Keyboard
@docs onKeyPress, onKeyDown, onKeyUp

# Mouse
@docs onClick, onMouseMove, onMouseDown, onMouseUp

# Window
@docs onResize, onVisibilityChange, Visibility

-}


import Browser.AnimationManager as AM
import Dict
import Elm.Kernel.Browser
import Json.Decode as Decode
import Process
import Task exposing (Task)
import Time



-- ANIMATION


{-| An animation frame triggers about 60 times per second. Get the POSIX time
on each frame. (See [`elm/time`](/packages/elm/time/latest) for more info on
POSIX times.)

**Note:** Browsers have their own render loop, repainting things as fast as
possible. If you want smooth animations in your application, it is helpful to
sync up with the browsers natural refresh rate. This hooks into JavaScript's
`requestAnimationFrame` function.
-}
onAnimationFrame : (Time.Posix -> msg) -> Sub msg
onAnimationFrame =
  AM.onAnimationFrame


{-| Just like `onAnimationFrame`, except message is the time in milliseconds
since the previous frame. So you should get a sequence of values all around
`1000 / 60` which is nice for stepping animations by a time delta.
-}
onAnimationFrameDelta : (Float -> msg) -> Sub msg
onAnimationFrameDelta =
  AM.onAnimationFrameDelta



-- KEYBOARD


{-| Subscribe to all key presses.

**Note:** Check out [this advice][note] to learn more about decoding key codes.
It is more complicated than it should be.

[note]: https://github.com/elm/browser/blob/1.0.0/notes/keyboard.md
-}
onKeyPress : Decode.Decoder msg -> Sub msg
onKeyPress =
  on Document "keypress"


{-| Subscribe to get codes whenever a key goes down. This can be useful for
creating games. Maybe you want to know if people are pressing `w`, `a`, `s`,
or `d` at any given time. Check out how that works in [this example][example].

**Note:** Check out [this advice][note] to learn more about decoding key codes.
It is more complicated than it should be.

[note]: https://github.com/elm/browser/blob/1.0.0/notes/keyboard.md
[example]: https://github.com/elm/browser/blob/1.0.0/examples/wasd.md
-}
onKeyDown : Decode.Decoder msg -> Sub msg
onKeyDown =
  on Document "keydown"


{-| Subscribe to get codes whenever a key goes up. Often used in combination
with [`onVisibilityChange`](#onVisibilityChange) to be sure keys do not appear
to down and never come back up.
-}
onKeyUp : Decode.Decoder msg -> Sub msg
onKeyUp =
  on Document "keyup"



-- MOUSE


{-| Subscribe to mouse clicks anywhere on screen. Maybe you need to create a
custom drop down. You could listen for clicks when it is open, letting you know
if someone clicked out of it:

    import Browser.Events as Events
    import Json.Decode as D

    type Msg = ClickOut

    subscriptions : Model -> Sub Msg
    subscriptions model =
      case model.dropDown of
        Closed _ ->
          Sub.none

        Open _ ->
          Events.onClick (D.succeed ClickOut)
-}
onClick : Decode.Decoder msg -> Sub msg
onClick =
  on Document "click"


{-| Subscribe to mouse moves anywhere on screen. You could use this to implement
drag and drop.

**Note:** Unsubscribe if you do not need these events! Running code on every
single mouse movement can be very costly, and it is recommended to only
subscribe when absolutely necessary.
-}
onMouseMove : Decode.Decoder msg -> Sub msg
onMouseMove =
  on Document "mousemove"


{-| Subscribe to get mouse information whenever the mouse button goes down.
-}
onMouseDown : Decode.Decoder msg -> Sub msg
onMouseDown =
  on Document "mousedown"


{-| Subscribe to get mouse information whenever the mouse button goes up.
Often used in combination with [`onVisibilityChange`](#onVisibilityChange)
to be sure keys do not appear to down and never come back up.
-}
onMouseUp : Decode.Decoder msg -> Sub msg
onMouseUp =
  on Document "mouseup"



-- WINDOW


{-| Subscribe to any changes in window size.

If you wanted to always track the current width, you could do something [like
this](TODO).

**Note:** This is equivalent to getting events from [`window.onresize`][resize].

[resize]: https://developer.mozilla.org/en-US/docs/Web/API/GlobalEventHandlers/onresize
-}
onResize : (Int -> Int -> msg) -> Sub msg
onResize func =
  on Window "resize" <|
    Decode.field "target" <|
      Decode.map2 func
        (Decode.field "innerWidth" Decode.int)
        (Decode.field "innerHeight" Decode.int)


{-| Subscribe to any visibility changes, like if the user switches to a
different tab or window. When the user looks away, you may want to:

- Stop polling a server for new information.
- Pause video or audio.
- Pause an image carousel.

This may also be useful with [`onKeyDown`](#onKeyDown). If you only listen for
[`onKeyUp`](#onKeyUp) to end the key press, you can miss situations like using
a keyboard shortcut to switch tabs. Visibility changes will cover those tricky
cases, like in [this example][example]!

[example]: https://github.com/elm/browser/blob/1.0.0/examples/wasd.md
-}
onVisibilityChange : (Visibility -> msg) -> Sub msg
onVisibilityChange func =
  let
    info = Elm.Kernel.Browser.visibilityInfo ()
  in
  on Document info.changes <|
    Decode.map (withHidden func) <|
      Decode.field "target" <|
        Decode.field info.hidden Decode.bool


withHidden : (Visibility -> msg) -> Bool -> msg
withHidden func isHidden =
  func (if isHidden then Hidden else Visible)


{-| Value describing whether the page is hidden or visible.
-}
type Visibility = Visible | Hidden



-- SUBSCRIPTIONS


type Node
  = Document
  | Window


on : Node -> String -> Decode.Decoder msg -> Sub msg
on node name decoder =
  subscription (MySub node name decoder)


type MySub msg =
  MySub Node String (Decode.Decoder msg)


subMap : (a -> b) -> MySub a -> MySub b
subMap func (MySub node name decoder) =
  MySub node name (Decode.map func decoder)



-- EFFECT MANAGER


type alias State msg =
  { subs : List (String, MySub msg)
  , pids : Dict.Dict String Process.Id
  }


init : Task Never (State msg)
init =
  Task.succeed (State [] Dict.empty)


type alias Event =
  { key : String
  , event : Decode.Value
  }


onSelfMsg : Platform.Router msg Event -> Event -> State msg -> Task Never (State msg)
onSelfMsg router { key, event } state =
  let
    toMessage (subKey, MySub node name decoder) =
      if subKey == key then
        Elm.Kernel.Browser.decodeEvent decoder event
      else
        Nothing

    messages =
      List.filterMap toMessage state.subs
  in
  Task.sequence (List.map (Platform.sendToApp router) messages)
    |> Task.andThen (\_ -> Task.succeed state)


onEffects : Platform.Router msg Event -> List (MySub msg) -> State msg -> Task Never (State msg)
onEffects router subs state =
  let
    newSubs =
      List.map addKey subs

    stepLeft _ pid (deads, lives, news) =
      ( pid :: deads, lives, news )

    stepBoth key pid _ (deads, lives, news) =
      ( deads, Dict.insert key pid lives, news )

    stepRight key sub (deads, lives, news) =
      ( deads, lives, spawn router key sub :: news )

    (deadPids, livePids, makeNewPids) =
      Dict.merge stepLeft stepBoth stepRight state.pids (Dict.fromList newSubs) ([], Dict.empty, [])
  in
  Task.sequence (List.map Process.kill deadPids)
    |> Task.andThen (\_ -> Task.sequence makeNewPids)
    |> Task.andThen (\pids -> Task.succeed (State newSubs (Dict.union livePids (Dict.fromList pids))))



-- TO KEY


addKey : MySub msg -> ( String, MySub msg )
addKey (MySub node name _ as sub) =
  ( nodeToKey node ++ name, sub )


nodeToKey : Node -> String
nodeToKey node =
  case node of
    Document ->
      "d_"

    Window ->
      "w_"



-- SPAWN


spawn : Platform.Router msg Event -> String -> MySub msg -> Task Never (String, Process.Id)
spawn router key (MySub node name _) =
  let
    actualNode =
      case node of
        Document ->
          Elm.Kernel.Browser.doc

        Window ->
          Elm.Kernel.Browser.window
  in
  Task.map (\value -> (key,value)) <|
    Elm.Kernel.Browser.on actualNode name <|
      \event -> Platform.sendToSelf router (Event key event)
