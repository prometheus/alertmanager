module Views.AlertList.AlertView exposing (addLabelMsg, view)

import Data.Alert exposing (Alert)
import Data.Alerts exposing (Alerts)
import Dict exposing (Dict)
import Html exposing (..)
import Html.Attributes exposing (class, href, readonly, style, title, value)
import Html.Events exposing (onClick)
import Types exposing (Msg(..))
import Utils.Date
import Utils.Filter
import Utils.Views
import Views.AlertList.Types exposing (AlertListMsg(..))
import Views.FilterBar.Types as FilterBarTypes
import Views.SilenceForm.Parsing exposing (newSilenceFromAlertLabels)


view : List ( String, String ) -> Maybe String -> Alert -> Html Msg
view labels maybeActiveId alert =
    let
        -- remove the grouping labels, and bring the alertname to front
        ungroupedLabels =
            alert.labels
                |> Dict.toList
                |> List.filter ((\b a -> List.member a b) labels >> not)
                |> List.partition (Tuple.first >> (==) "alertname")
                |> (\( a, b ) -> (++) a b)
    in
    li
        [ -- speedup rendering in Chrome, because list-group-item className
          -- creates a new layer in the rendering engine
          style "position" "static"
        , class "align-items-start list-group-item border-0 p-0 mb-4"
        ]
        [ div
            [ class "w-100 mb-2 d-flex align-items-start" ]
            [ titleView alert
            , case alert.annotations of
                Just a ->
                    if List.length (Dict.toList a) > 0 then
                        annotationsButton maybeActiveId alert

                    else
                        text ""

                Nothing ->
                    text ""
            , generatorUrlButton alert.generatorURL
            , silenceButton alert
            ]

        -- TODO: Moving activeId to fingerprint. Is that correct?
        , if maybeActiveId == alert.fingerprint then
            annotations alert.annotations

          else
            text ""
        , div [] (List.map labelButton ungroupedLabels)
        ]


annotations : Maybe (Dict String String) -> Html Msg
annotations maybeAnnotations =
    case maybeAnnotations of
        Just a ->
            table [ class "table w-100 mb-1" ] (List.map annotation <| Dict.toList a)

        Nothing ->
            text ""


titleView : Alert -> Html Msg
titleView { startsAt, status } =
    case status of
        Just status_ ->
            let
                ( className, inhibited ) =
                    if not <| List.isEmpty status_.inhibitedBy then
                        ( "text-muted", " (inhibited)" )

                    else
                        ( "", "" )
            in
            span
                [ class ("align-self-center mr-2 " ++ className) ]
                [ text
                    (Maybe.withDefault "" (Maybe.map Utils.Date.dateTimeFormat startsAt)
                        ++ inhibited
                    )
                ]

        Nothing ->
            -- TODO: What to return here? This case should never happen.
            span [] []


annotationsButton : Maybe String -> Alert -> Html Msg
annotationsButton maybeActiveId alert =
    -- TODO: Moving activeId to fingerprint. Is that correct?
    if maybeActiveId == alert.fingerprint then
        button
            [ onClick (SetActive Nothing |> MsgForAlertList)
            , class "btn btn-outline-info border-0 active"
            ]
            [ i [ class "fa fa-minus mr-2" ] [], text "Info" ]

    else
        button
            -- TODO: Moving activeId to fingerprint. Is that correct?
            [ onClick (SetActive alert.fingerprint |> MsgForAlertList)
            , class "btn btn-outline-info border-0"
            ]
            [ i [ class "fa fa-plus mr-2" ] [], text "Info" ]


annotation : ( String, String ) -> Html Msg
annotation ( key, value ) =
    tr []
        [ th [ class "text-nowrap" ] [ text (key ++ ":") ]
        , td [ class "w-100" ] (Utils.Views.linkifyText value)
        ]


labelButton : ( String, String ) -> Html Msg
labelButton ( key, val ) =
    div
        [ class "btn-group mr-2 mb-2" ]
        [ span
            [ class "btn btn-sm border-right-0 text-muted"

            -- have to reset bootstrap button styles to make the text selectable
            , style "user-select" "initial"

            -- have to reset bootstrap button styles to make the text selectable
            , style "-moz-user-select" "initial"

            -- have to reset bootstrap button styles to make the text selectable
            , style "-webkit-user-select" "initial"

            -- have to reset bootstrap button styles to make the text selectable
            , style "border-color" "#ccc"
            ]
            [ text (key ++ "=\"" ++ val ++ "\"") ]
        , button
            [ class "btn btn-sm bg-faded btn-outline-secondary"
            , onClick (addLabelMsg ( key, val ))
            , title "Filter by this label"
            ]
            [ text "+" ]
        ]


addLabelMsg : ( String, String ) -> Msg
addLabelMsg ( key, value ) =
    FilterBarTypes.AddFilterMatcher False
        { key = key
        , op = Utils.Filter.Eq
        , value = value
        }
        |> MsgForFilterBar
        |> MsgForAlertList


silenceButton : Alert -> Html Msg
silenceButton alert =
    case alert.status of
        Just status ->
            case List.head status.silencedBy of
                Just sId ->
                    a
                        [ class "btn btn-outline-danger border-0"
                        , href ("#/silences/" ++ sId)
                        ]
                        [ i [ class "fa fa-bell-slash mr-2" ] []
                        , text "Silenced"
                        ]

                Nothing ->
                    a
                        [ class "btn btn-outline-info border-0"
                        , href (newSilenceFromAlertLabels alert.labels)
                        ]
                        [ i [ class "fa fa-bell-slash-o mr-2" ] []
                        , text "Silence"
                        ]

        Nothing ->
            -- TODO: What to return here? This case should never happen.
            a [] [ text "Alert has no status" ]


generatorUrlButton : Maybe String -> Html Msg
generatorUrlButton maybeUrl =
    case maybeUrl of
        Just url ->
            a
                [ class "btn btn-outline-info border-0", href url ]
                [ i [ class "fa fa-line-chart mr-2" ] []
                , text "Source"
                ]

        Nothing ->
            text ""
