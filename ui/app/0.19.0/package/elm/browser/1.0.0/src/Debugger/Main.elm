module Debugger.Main exposing
  ( wrapInit
  , wrapUpdate
  , wrapSubs
  , getUserModel
  , cornerView
  , popoutView
  )


import Elm.Kernel.Debugger
import Json.Decode as Decode
import Json.Encode as Encode
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onClick)
import Task exposing (Task)
import Debugger.Expando as Expando exposing (Expando)
import Debugger.History as History exposing (History)
import Debugger.Metadata as Metadata exposing (Metadata)
import Debugger.Overlay as Overlay
import Debugger.Report as Report



-- VIEW


getUserModel : Model model msg -> model
getUserModel model =
  getCurrentModel model.state



-- SUBSCRIPTIONS


wrapSubs : (model -> Sub msg) -> Model model msg -> Sub (Msg msg)
wrapSubs subscriptions model =
  Sub.map UserMsg (subscriptions (getLatestModel model.state))



-- MODEL


type alias Model model msg =
  { history : History model msg
  , state : State model
  , expando : Expando
  , metadata : Result Metadata.Error Metadata
  , overlay : Overlay.State
  , popout : Popout
  }


type Popout = Popout Popout



-- STATE


type State model
  = Running model
  | Paused Int model model


getLatestModel : State model -> model
getLatestModel state =
  case state of
    Running model ->
      model

    Paused _ _ model ->
      model


getCurrentModel : State model -> model
getCurrentModel state =
  case state of
    Running model ->
      model

    Paused _ model _ ->
      model


isPaused : State model -> Bool
isPaused state =
  case state of
    Running _ ->
      False

    Paused _ _ _ ->
      True



-- INIT


wrapInit : Encode.Value -> Popout -> (flags -> (model, Cmd msg)) -> flags -> (Model model msg, Cmd (Msg msg))
wrapInit metadata popout init flags =
  let
    (userModel, userCommands) =
      init flags
  in
  ( { history = History.empty userModel
    , state = Running userModel
    , expando = Expando.init userModel
    , metadata = Metadata.decode metadata
    , overlay = Overlay.none
    , popout = popout
    }
  , Cmd.map UserMsg userCommands
  )



-- UPDATE


type Msg msg
  = NoOp
  | UserMsg msg
  | ExpandoMsg Expando.Msg
  | Resume
  | Jump Int
  | Open
  | Up
  | Down
  | Import
  | Export
  | Upload String
  | OverlayMsg Overlay.Msg


type alias UserUpdate model msg =
  msg -> model -> ( model, Cmd msg )


wrapUpdate : UserUpdate model msg -> Msg msg -> Model model msg -> (Model model msg, Cmd (Msg msg))
wrapUpdate update msg model =
  case msg of
    NoOp ->
      ( model, Cmd.none )

    UserMsg userMsg ->
      let
        userModel = getLatestModel model.state
        newHistory = History.add userMsg userModel model.history
        (newUserModel, userCmds) = update userMsg userModel
        commands = Cmd.map UserMsg userCmds
      in
      case model.state of
        Running _ ->
          ( { model
              | history = newHistory
              , state = Running newUserModel
              , expando = Expando.merge newUserModel model.expando
            }
          , Cmd.batch [ commands, scroll model.popout ]
          )

        Paused index indexModel _ ->
          ( { model
              | history = newHistory
              , state = Paused index indexModel newUserModel
            }
          , commands
          )

    ExpandoMsg eMsg ->
      ( { model | expando = Expando.update eMsg model.expando }
      , Cmd.none
      )

    Resume ->
      case model.state of
        Running _ ->
          ( model, Cmd.none )

        Paused _ _ userModel ->
          ( { model
              | state = Running userModel
              , expando = Expando.merge userModel model.expando
            }
          , scroll model.popout
          )

    Jump index ->
      let
        (indexModel, indexMsg) =
          History.get update index model.history
      in
      ( { model
          | state = Paused index indexModel (getLatestModel model.state)
          , expando = Expando.merge indexModel model.expando
        }
      , Cmd.none
      )

    Open ->
      ( model
      , Task.perform (\_ -> NoOp) (Elm.Kernel.Debugger.open model.popout)
      )

    Up ->
      let
        index =
          case model.state of
            Paused i _ _ ->
              i

            Running _ ->
              History.size model.history
      in
      if index > 0 then
        wrapUpdate update (Jump (index - 1)) model
      else
        ( model, Cmd.none )

    Down ->
      case model.state of
        Running _ ->
          ( model, Cmd.none )

        Paused index _ userModel ->
          if index == History.size model.history - 1 then
            wrapUpdate update Resume model
          else
            wrapUpdate update (Jump (index + 1)) model

    Import ->
      withGoodMetadata model <| \_ ->
        ( model, upload )

    Export ->
      withGoodMetadata model <| \metadata ->
        ( model, download metadata model.history )

    Upload jsonString ->
      withGoodMetadata model <| \metadata ->
        case Overlay.assessImport metadata jsonString of
          Err newOverlay ->
            ( { model | overlay = newOverlay }, Cmd.none )

          Ok rawHistory ->
            loadNewHistory rawHistory update model

    OverlayMsg overlayMsg ->
      case Overlay.close overlayMsg model.overlay of
        Nothing ->
          ( { model | overlay = Overlay.none }, Cmd.none )

        Just rawHistory ->
          loadNewHistory rawHistory update model



-- COMMANDS


scroll : Popout -> Cmd (Msg msg)
scroll popout =
  Task.perform (always NoOp) (Elm.Kernel.Debugger.scroll popout)


upload : Cmd (Msg msg)
upload =
  Task.perform Upload (Elm.Kernel.Debugger.upload ())


download : Metadata -> History model msg -> Cmd (Msg msg)
download metadata history =
  let
    historyLength =
      History.size history

    json =
      Encode.object
        [ ("metadata", Metadata.encode metadata)
        , ("history", History.encode history)
        ]
  in
    Task.perform (\_ -> NoOp) (Elm.Kernel.Debugger.download historyLength json)



-- UPDATE OVERLAY


withGoodMetadata
  : Model model msg
  -> (Metadata -> (Model model msg, Cmd (Msg msg)))
  -> (Model model msg, Cmd (Msg msg))
withGoodMetadata model func =
  case model.metadata of
    Ok metadata ->
      func metadata

    Err error ->
      ( { model | overlay = Overlay.badMetadata error }
      , Cmd.none
      )


loadNewHistory
  : Encode.Value
  -> UserUpdate model msg
  -> Model model msg
  -> ( Model model msg, Cmd (Msg msg) )
loadNewHistory rawHistory update model =
  let
    initialUserModel =
      History.getInitialModel model.history

    pureUserUpdate msg userModel =
      Tuple.first (update msg userModel)

    decoder =
      History.decoder initialUserModel pureUserUpdate
  in
  case Decode.decodeValue decoder rawHistory of
    Err _ ->
      ( { model | overlay = Overlay.corruptImport }
      , Cmd.none
      )

    Ok (latestUserModel, newHistory) ->
      ( { model
          | history = newHistory
          , state = Running latestUserModel
          , expando = Expando.init latestUserModel
          , overlay = Overlay.none
        }
      , Cmd.none
      )



-- CORNER VIEW


cornerView : Model model msg -> Html (Msg msg)
cornerView model =
  Overlay.view
    { resume = Resume
    , open = Open
    , importHistory = Import
    , exportHistory = Export
    , wrap = OverlayMsg
    }
    (isPaused model.state)
    (Elm.Kernel.Debugger.isOpen model.popout)
    (History.size model.history)
    model.overlay


toBlockerType : Model model msg -> Overlay.BlockerType
toBlockerType model =
  Overlay.toBlockerType (isPaused model.state) model.overlay



-- BIG DEBUG VIEW


popoutView : Model model msg -> Html (Msg msg)
popoutView { history, state, expando } =
  node "body"
    [ style "margin" "0"
    , style "padding" "0"
    , style "width" "100%"
    , style "height" "100%"
    , style "font-family" "monospace"
    , style "overflow" "auto"
    ]
    [ viewSidebar state history
    , Html.map ExpandoMsg <|
        div
          [ style "display" "block"
          , style "float" "left"
          , style "height" "100%"
          , style "width" "calc(100% - 30ch)"
          , style "margin" "0"
          , style "overflow" "auto"
          , style "cursor" "default"
          ]
          [ Expando.view Nothing expando
          ]
    ]


viewSidebar : State model -> History model msg -> Html (Msg msg)
viewSidebar state history =
  let
    maybeIndex =
      case state of
        Running _ ->
          Nothing

        Paused index _ _ ->
          Just index
  in
    div
      [ style "display" "block"
      , style "float" "left"
      , style "width" "30ch"
      , style "height" "100%"
      , style "color" "white"
      , style "background-color" "rgb(61, 61, 61)"
      ]
      [ Html.map Jump (History.view maybeIndex history)
      , playButton maybeIndex
      ]


playButton : Maybe Int -> Html (Msg msg)
playButton maybeIndex =
  div
    [ style "width" "100%"
    , style "text-align" "center"
    , style "background-color" "rgb(50, 50, 50)"
    ]
    [ viewResumeButton maybeIndex
    , div
        [ style "width" "100%"
        , style "height" "24px"
        , style "line-height" "24px"
        , style "font-size" "12px"
        ]
        [ viewTextButton Import "Import"
        , text " / "
        , viewTextButton Export "Export"
        ]
    ]


viewTextButton : msg -> String -> Html msg
viewTextButton msg label =
  span
    [ onClick msg
    , style "cursor" "pointer"
    ]
    [ text label ]


viewResumeButton : Maybe Int -> Html (Msg msg)
viewResumeButton maybeIndex =
  case maybeIndex of
    Nothing ->
      text ""

    Just _ ->
      div
        [ onClick Resume
        , class "elm-debugger-resume"
        ]
        [ text "Resume"
        , Html.node "style" [] [ text resumeStyle ]
        ]


resumeStyle : String
resumeStyle = """

.elm-debugger-resume {
  width: 100%;
  height: 30px;
  line-height: 30px;
  cursor: pointer;
}

.elm-debugger-resume:hover {
  background-color: rgb(41, 41, 41);
}

"""
