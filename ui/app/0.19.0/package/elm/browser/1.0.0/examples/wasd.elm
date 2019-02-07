
import Browser.Keyboard as Keyboard
import Browser.Window as Window
import Json.Decode as D



-- MODEL


type alias Model =
  { x : Float
  , y : Float
  , north : KeyStatus
  , south : KeyStatus
  , east : KeyStatus
  , west : KeyStatus
  }


type KeyStatus = Up | Down


init : () -> ( Model, Cmd Msg )
init _ =
  ( Model 0 0 Up Up Up Up
  , Cmd.none
  )



-- UPDATE


type Msg
  = Change KeyStatus String
  | Blur
  | TimeDelta Float


update : Msg -> Model -> (Model, Cmd Msg)
update msg model =
  case msg of
    Change status string ->
      ( updateKey status string
      , Cmd.none
      )

    Blur ->
      ( Model model.x model.y Up Up Up Up
      , Cmd.none
      )

    TimeDelta delta ->
      ( updatePosition delta model
      , Cmd.none
      )


updateKey : KeyStatus -> String -> Model -> Model
updateKey status string model =
  case string of
    "w" -> { model | north = status }
    "a" -> { model | east  = status }
    "s" -> { model | south = status }
    "d" -> { model | west  = status }
    _   -> model


updatePosition : Float -> Model -> Model
updatePosition delta model =
  let
    vx = toOne model.east - toOne model.west
    vy = toOne model.north - toOne model.south
  in
  { model
      | x = model.x + vx * delta
      , y = model.y + vy * delta
  }


toOne : KeyStatus -> Float
toOne status =
  if isDown status then 1 else 0


isDown : KeyStatus -> Bool
isDown status =
  case status of
    Down -> True
    Up -> False



-- SUBSCRIPTIONS


subscriptions : Model -> Sub Msg
subscriptions model =
  Sub.batch
    [ Keyboard.downs (D.map (Change Down) keyDecoder)
    , Keyboard.ups (D.map (Change Up) keyDecoder)
    , Window.blurs (D.succeed Blur)
    , if anyIsDown then
        Animation.deltas TimeDelta
      else
        Sub.none
    ]


keyDecoder : D.Decoder String
keyDecoder =
  D.field "key" D.string


anyIsDown : Model -> Bool
anyIsDown model =
  isDown model.north
  || isDown model.south
  || isDown model.east
  || isDown model.west



-- VIEW


view : Model -> Html Msg
view model =
  div
    [ style "background-color" "rgb(104,216,239)"
    , style "position" "absolute"
    , style "top"  (String.fromInt (round model.x) ++ "px")
    , style "left" (String.fromInt (round model.y) ++ "px")
    , style "width" "100px"
    , style "height" "100px"
    ]
    [ text "Press WASD keys!"
    ]
