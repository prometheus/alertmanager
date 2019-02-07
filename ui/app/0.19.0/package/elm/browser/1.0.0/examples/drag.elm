import Browser.Mouse as Mouse
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)



-- MAIN


main =
  Browser.element
    { init = init
    , update = update
    , subscriptions = subscriptions
    , view = view
    }


-- MODEL


type Model =
  { x : Int
  , y : Int
  , dragState : DragState
  }


type DragState
  = Static
  | Moving Int Int Int Int


init : () -> (Model, Cmd Msg)
init _ =
  ( Model 100 100 Static
  , Cmd.none
  )



-- UPDATE


type Msg
  = Start Int Int
  | Move Int Int
  | Stop Int Int


update : Msg -> Model -> (Model, Cmd Msg)
update msg model =
  case msg of
    Start x y ->
      ( { model | dragState = Moving x y x y }
      , Cmd.none
      )

    Move x y ->
      case model.dragState of
        Static ->
          ( model, Cmd.none )

        Moving startX startY _ _ ->
          ( { model | dragState = Moving startX startY x y }
          )

    Stop x y ->
      case model.dragState of
        Static ->
          ( model, Cmd.none )

        Moving startX startY _ _ ->
          ( Model (model.x + startX - x) (model.y + startY - y) Static
          , Cmd.none
          )



-- VIEW


view : Model -> Html Msg
view model =
  let
    (x, y) = getPosition model
  in
  div
    [ style "background-color" "rgb(104,216,239)"
    , style "position" "absolute"
    , style "top"  (String.fromInt x ++ "px")
    , style "left" (String.fromInt y ++ "px")
    , style "width" "100px"
    , style "height" "100px"
    , on "mousedown" (D.map2 Start pageX pageY)
    , on "mouseup" (D.map2 Stop pageX pageY)
    ]
    [ text "Drag me!"
    ]


getPosition : Model -> (Int, Int)
getPosition model =
  case model.dragState of
    Static ->
      (model.x, model.y)

    Moving startX startY endX endY ->
      (x + startX - endX, y + startY - endY)



-- SUBSCRIPTIONS


subscriptions : Model -> Sub Msg
subscriptions model =
  case model.dragState of
    Static ->
      Sub.none

    Moving _ _ _ _ ->
      Mouse.moves (D.map2 Move pageX pageY)


pageX : D.Decoder Int
pageX =
  D.field "pageX" D.int


pageY : D.Decoder Int
pageY =
  D.field "pageY" D.int
