effect module Browser.AnimationManager where { subscription = MySub } exposing
  ( onAnimationFrame
  , onAnimationFrameDelta
  )


import Elm.Kernel.Browser
import Process
import Task exposing (Task)
import Time



-- PUBLIC STUFF


onAnimationFrame : (Time.Posix -> msg) -> Sub msg
onAnimationFrame tagger =
  subscription (Time tagger)


onAnimationFrameDelta : (Float -> msg) -> Sub msg
onAnimationFrameDelta tagger =
  subscription (Delta tagger)



-- SUBSCRIPTIONS


type MySub msg
  = Time (Time.Posix -> msg)
  | Delta (Float -> msg)


subMap : (a -> b) -> MySub a -> MySub b
subMap func sub =
  case sub of
    Time tagger ->
      Time (func << tagger)

    Delta tagger ->
      Delta (func << tagger)



-- EFFECT MANAGER


type alias State msg =
  { subs : List (MySub msg)
  , request : Maybe Process.Id
  , oldTime : Int
  }


-- NOTE: used in onEffects
--
init : Task Never (State msg)
init =
  Task.succeed (State [] Nothing 0)


onEffects : Platform.Router msg Int -> List (MySub msg) -> State msg -> Task Never (State msg)
onEffects router subs {request, oldTime} =
  case (request, subs) of
    (Nothing, []) ->
      init

    (Just pid, []) ->
      Process.kill pid
        |> Task.andThen (\_ -> init)

    (Nothing, _) ->
      Process.spawn (Task.andThen (Platform.sendToSelf router) rAF)
        |> Task.andThen (\pid -> now
        |> Task.andThen (\time -> Task.succeed (State subs (Just pid) time)))

    (Just _, _) ->
      Task.succeed (State subs request oldTime)


onSelfMsg : Platform.Router msg Int -> Int -> State msg -> Task Never (State msg)
onSelfMsg router newTime {subs, oldTime} =
  let
    send sub =
      case sub of
        Time tagger ->
          Platform.sendToApp router (tagger (Time.millisToPosix newTime))

        Delta tagger ->
          Platform.sendToApp router (tagger (toFloat (newTime - oldTime)))
  in
  Process.spawn (Task.andThen (Platform.sendToSelf router) rAF)
    |> Task.andThen (\pid -> Task.sequence (List.map send subs)
    |> Task.andThen (\_ -> Task.succeed (State subs (Just pid) newTime)))


rAF : Task x Int
rAF =
  Elm.Kernel.Browser.rAF ()


now : Task x Int
now =
  Elm.Kernel.Browser.now ()