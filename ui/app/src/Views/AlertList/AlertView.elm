module Views.AlertList.AlertView exposing (view, addLabelMsg)

import Alerts.Types exposing (Alert)
import Html exposing (..)
import Html.Attributes exposing (class, style, href, value, readonly, title)
import Html.Events exposing (onClick)
import Types exposing (Msg(Noop, MsgForAlertList))
import Utils.Date
import Views.FilterBar.Types as FilterBarTypes
import Views.AlertList.Types exposing (AlertListMsg(MsgForFilterBar, SetActive))
import Utils.Views
import Utils.Filter
import Views.SilenceForm.Parsing exposing (newSilenceFromAlertLabels)


view : List ( String, String ) -> Maybe String -> Alert -> Html Msg
view labels maybeActiveId alert =
    let
        -- remove the grouping labels, and bring the alertname to front
        ungroupedLabels =
            alert.labels
                |> List.filter ((flip List.member) labels >> not)
                |> List.partition (Tuple.first >> (==) "alertname")
                |> uncurry (++)
    in
        li
            [ -- speedup rendering in Chrome, because list-group-item className
              -- creates a new layer in the rendering engine
              style [ ( "position", "static" ) ]
            , class "align-items-start list-group-item border-0 p-0 mb-4"
            ]
            [ div
                [ class "w-100 mb-2 d-flex align-items-start" ]
                [ titleView alert
                , if List.length alert.annotations > 0 then
                    annotationsButton maybeActiveId alert
                  else
                    text ""
                , generatorUrlButton alert.generatorUrl
                , silenceButton alert
                ]
            , if maybeActiveId == Just alert.id then
                table [ class "table w-100 mb-1" ] (List.map annotation alert.annotations)
              else
                text ""
            , div [] (List.map labelButton ungroupedLabels)
            ]


titleView : Alert -> Html Msg
titleView { startsAt, isInhibited } =
    let
        ( className, inhibited ) =
            if isInhibited then
                ( "text-muted", " (inhibited)" )
            else
                ( "", "" )
    in
        span
            [ class ("align-self-center mr-2 " ++ className) ]
            [ text
                (Utils.Date.timeFormat startsAt
                    ++ ", "
                    ++ Utils.Date.dateFormat startsAt
                    ++ inhibited
                )
            ]


annotationsButton : Maybe String -> Alert -> Html Msg
annotationsButton maybeActiveId alert =
    if maybeActiveId == Just alert.id then
        button
            [ onClick (SetActive Nothing |> MsgForAlertList)
            , class "btn btn-outline-info border-0 active"
            ]
            [ i [ class "fa fa-minus mr-2" ] [], text "Info" ]
    else
        button
            [ onClick (SetActive (Just alert.id) |> MsgForAlertList)
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
            , style
                [ ( "user-select", "initial" )
                , ( "-moz-user-select", "initial" )
                , ( "-webkit-user-select", "initial" )
                , ( "border-color", "#ccc" )
                ]
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
    case alert.silenceId of
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


generatorUrlButton : String -> Html Msg
generatorUrlButton url =
    a
        [ class "btn btn-outline-info border-0", href url ]
        [ i [ class "fa fa-line-chart mr-2" ] []
        , text "Source"
        ]
