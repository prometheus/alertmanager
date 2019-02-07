module Debugger.Overlay exposing
  ( State, none, corruptImport, badMetadata
  , Msg, close, assessImport
  , BlockerType(..), toBlockerType
  , Config
  , view
  , viewImportExport
  )

import Json.Decode as Decode
import Json.Encode as Encode
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onClick)
import Debugger.Metadata as Metadata exposing (Metadata)
import Debugger.Report as Report exposing (Report)



type State
  = None
  | BadMetadata Metadata.Error
  | BadImport Report
  | RiskyImport Report Encode.Value


none : State
none =
  None


corruptImport : State
corruptImport =
  BadImport Report.CorruptHistory


badMetadata : Metadata.Error -> State
badMetadata =
  BadMetadata



--  UPDATE


type Msg = Cancel | Proceed


close : Msg -> State -> Maybe Encode.Value
close msg state =
  case state of
    None ->
      Nothing

    BadMetadata _ ->
      Nothing

    BadImport _ ->
      Nothing

    RiskyImport _ rawHistory ->
      case msg of
        Cancel ->
          Nothing

        Proceed ->
          Just rawHistory


assessImport : Metadata -> String -> Result State Encode.Value
assessImport metadata jsonString =
  case Decode.decodeString uploadDecoder jsonString of
    Err _ ->
      Err corruptImport

    Ok (foreignMetadata, rawHistory) ->
      let
        report =
          Metadata.check foreignMetadata metadata
      in
        case Report.evaluate report of
          Report.Impossible ->
            Err (BadImport report)

          Report.Risky ->
            Err (RiskyImport report rawHistory)

          Report.Fine ->
            Ok rawHistory


uploadDecoder : Decode.Decoder (Metadata, Encode.Value)
uploadDecoder =
  Decode.map2 (\x y -> (x,y))
    (Decode.field "metadata" Metadata.decoder)
    (Decode.field "history" Decode.value)



-- BLOCKERS


type BlockerType = BlockNone | BlockMost | BlockAll


toBlockerType : Bool -> State -> BlockerType
toBlockerType isPaused state =
  case state of
    None            -> if isPaused then BlockAll else BlockNone
    BadMetadata _   -> BlockMost
    BadImport _     -> BlockMost
    RiskyImport _ _ -> BlockMost



-- VIEW


type alias Config msg =
  { resume : msg
  , open : msg
  , importHistory : msg
  , exportHistory : msg
  , wrap : Msg -> msg
  }


view : Config msg -> Bool -> Bool -> Int -> State -> Html msg
view config isPaused isOpen numMsgs state =
  case state of
    None ->
      if isOpen then
        text ""

      else
        if isPaused then
          div
            [ style "width" "100%"
            , style "height" "100%"
            , style "cursor" "pointer"
            , style "text-align" "center"
            , style "pointer-events" "auto"
            , style "background-color" "rgba(200, 200, 200, 0.7)"
            , style "color" "white"
            , style "font-family" "'Trebuchet MS', 'Lucida Grande', 'Bitstream Vera Sans', 'Helvetica Neue', sans-serif"
            , style "z-index" "2147483646"
            , onClick config.resume
            ]
            [ div
                [ style "position" "absolute"
                , style "top" "calc(50% - 40px)"
                , style "font-size" "80px"
                , style "line-height" "80px"
                , style "height" "80px"
                , style "width" "100%"
                ]
                [ text "Click to Resume"
                ]
            , viewMiniControls config numMsgs
            ]
        else
          viewMiniControls config numMsgs

    BadMetadata badMetadata_ ->
      viewMessage config
        "Cannot use Import or Export"
        (viewBadMetadata badMetadata_)
        (Accept "Ok")

    BadImport report ->
      viewMessage config
        "Cannot Import History"
        (viewReport True report)
        (Accept "Ok")

    RiskyImport report _ ->
      viewMessage config
        "Warning"
        (viewReport False report)
        (Choose "Cancel" "Import Anyway")



-- VIEW MESSAGE


viewMessage : Config msg -> String -> List (Html msg) -> Buttons -> Html msg
viewMessage config title details buttons =
  div
    [ id "elm-debugger-overlay"
    , style "position" "fixed"
    , style "top" "0"
    , style "left" "0"
    , style "width" "100%"
    , style "height" "100%"
    , style "color" "white"
    , style "pointer-events" "none"
    , style "font-family" "'Trebuchet MS', 'Lucida Grande', 'Bitstream Vera Sans', 'Helvetica Neue', sans-serif"
    , style "z-index" "2147483647"
    ]
    [ div
        [ style "position" "absolute"
        , style "width" "600px"
        , style "height" "100%"
        , style "padding-left" "calc(50% - 300px)"
        , style "padding-right" "calc(50% - 300px)"
        , style "background-color" "rgba(200, 200, 200, 0.7)"
        , style "pointer-events" "auto"
        ]
        [ div
            [ style "font-size" "36px"
            , style "height" "80px"
            , style "background-color" "rgb(50, 50, 50)"
            , style "padding-left" "22px"
            , style "vertical-align" "middle"
            , style "line-height" "80px"
            ]
            [ text title ]
        , div
            [ id "elm-debugger-details"
            , style "padding" " 8px 20px"
            , style "overflow-y" "auto"
            , style "max-height" "calc(100% - 156px)"
            , style "background-color" "rgb(61, 61, 61)"
            ]
            details
        , Html.map config.wrap (viewButtons buttons)
        ]
    ]


viewReport : Bool -> Report -> List (Html msg)
viewReport isBad report =
  case report of
    Report.CorruptHistory ->
      [ text "Looks like this history file is corrupt. I cannot understand it."
      ]

    Report.VersionChanged old new ->
      [ text <|
          "This history was created with Elm "
          ++ old ++ ", but you are using Elm "
          ++ new ++ " right now."
      ]

    Report.MessageChanged old new ->
      [ text <|
          "To import some other history, the overall message type must"
          ++ " be the same. The old history has "
      , viewCode old
      , text " messages, but the new program works with "
      , viewCode new
      , text " messages."
      ]

    Report.SomethingChanged changes ->
      [ p [] [ text (if isBad then explanationBad else explanationRisky) ]
      , ul
          [ style "list-style-type" "none"
          , style "padding-left" "20px"
          ]
          (List.map viewChange changes)
      ]


explanationBad : String
explanationBad = """
The messages in this history do not match the messages handled by your
program. I noticed changes in the following types:
"""

explanationRisky : String
explanationRisky = """
This history seems old. It will work with this program, but some
messages have been added since the history was created:
"""


viewCode : String -> Html msg
viewCode name =
  code [] [ text name ]


viewChange : Report.Change -> Html msg
viewChange change =
  li [ style "margin" "8px 0" ] <|
    case change of
      Report.AliasChange name ->
        [ span [ style "font-size" "1.5em" ] [ viewCode name ]
        ]

      Report.UnionChange name { removed, changed, added, argsMatch } ->
        [ span [ style "font-size" "1.5em" ] [ viewCode name ]
        , ul
            [ style "list-style-type" "disc"
            , style "padding-left" "2em"
            ]
            [ viewMention removed "Removed "
            , viewMention changed "Changed "
            , viewMention added "Added "
            ]
        , if argsMatch then
            text ""
          else
            text "This may be due to the fact that the type variable names changed."
        ]


viewMention : List String -> String -> Html msg
viewMention tags verbed =
  case List.map viewCode (List.reverse tags) of
    [] ->
      text ""

    [tag] ->
      li []
        [ text verbed, tag, text "." ]

    [tag2, tag1] ->
      li []
        [ text verbed, tag1, text " and ", tag2, text "." ]

    lastTag :: otherTags ->
      li [] <|
        text verbed
        :: List.intersperse (text ", ") (List.reverse otherTags)
        ++ [ text ", and ", lastTag, text "." ]


viewBadMetadata : Metadata.Error -> List (Html msg)
viewBadMetadata {message, problems} =
  [ p []
      [ text "The "
      , viewCode message
      , text " type of your program cannot be reliably serialized for history files."
      ]
  , p [] [ text "Functions cannot be serialized, nor can values that contain functions. This is a problem in these places:" ]
  , ul [] (List.map viewProblemType problems)
  , p []
      [ text goodNews1
      , a [ href "https://guide.elm-lang.org/types/union_types.html" ] [ text "union types" ]
      , text ", in your messages. From there, your "
      , viewCode "update"
      , text goodNews2
      ]
  ]


goodNews1 = """
The good news is that having values like this in your message type is not
so great in the long run. You are better off using simpler data, like
"""


goodNews2 = """
function can pattern match on that data and call whatever functions, JSON
decoders, etc. you need. This makes the code much more explicit and easy to
follow for other readers (or you in a few months!)
"""


viewProblemType : Metadata.ProblemType -> Html msg
viewProblemType { name, problems } =
  li []
    [ viewCode name
    , text (" can contain " ++ addCommas (List.map problemToString problems) ++ ".")
    ]


problemToString : Metadata.Problem -> String
problemToString problem =
  case problem of
    Metadata.Function ->
      "functions"

    Metadata.Decoder ->
      "JSON decoders"

    Metadata.Task ->
      "tasks"

    Metadata.Process ->
      "processes"

    Metadata.Socket ->
      "web sockets"

    Metadata.Request ->
      "HTTP requests"

    Metadata.Program ->
      "programs"

    Metadata.VirtualDom ->
      "virtual DOM values"


addCommas : List String -> String
addCommas items =
  case items of
    [] ->
      ""

    [item] ->
      item

    [item1, item2] ->
      item1 ++ " and " ++ item2

    lastItem :: otherItems ->
      String.join ", " (otherItems ++ [ " and " ++ lastItem ])



-- VIEW MESSAGE BUTTONS


type Buttons
  = Accept String
  | Choose String String


viewButtons : Buttons -> Html Msg
viewButtons buttons =
  let
    btn msg string =
      Html.button
        [ style "margin-right" "20px"
        , onClick msg
        ]
        [ text string ]

    buttonNodes =
      case buttons of
        Accept proceed ->
          [ btn Proceed proceed
          ]

        Choose cancel proceed ->
          [ btn Cancel cancel
          , btn Proceed proceed
          ]
  in
  div
    [ style "height" "60px"
    , style "line-height" "60px"
    , style "text-align" "right"
    , style "background-color" "rgb(50, 50, 50)"
    ]
    buttonNodes



-- VIEW MINI CONTROLS


viewMiniControls : Config msg -> Int -> Html msg
viewMiniControls config numMsgs =
  div
    [ style "position" "fixed"
    , style "bottom" "0"
    , style "right" "6px"
    , style "border-radius" "4px"
    , style "background-color" "rgb(61, 61, 61)"
    , style "color" "white"
    , style "font-family" "monospace"
    , style "pointer-events" "auto"
    , style "z-index" "2147483647"
    ]
    [ div
        [ style "padding" "6px"
        , style "cursor" "pointer"
        , style "text-align" "center"
        , style "min-width" "24ch"
        , onClick config.open
        ]
        [ text ("Explore History (" ++ String.fromInt numMsgs ++ ")")
        ]
    , viewImportExport
        [ style "padding" "4px 0"
        , style "font-size" "0.8em"
        , style "text-align" "center"
        , style "background-color" "rgb(50, 50, 50)"
        ]
        config.importHistory
        config.exportHistory
    ]


viewImportExport : List (Attribute msg) -> msg -> msg -> Html msg
viewImportExport props importMsg exportMsg =
  div
    props
    [ button importMsg "Import"
    , text " / "
    , button exportMsg "Export"
    ]


button : msg -> String -> Html msg
button msg label =
  span [ onClick msg, style "cursor" "pointer" ] [ text label ]
