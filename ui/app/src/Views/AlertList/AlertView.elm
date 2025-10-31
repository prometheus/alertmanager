module Views.AlertList.AlertView exposing (addLabelMsg, view)

import Data.GettableAlert exposing (GettableAlert)
import Dict
import Html exposing (..)
import Html.Attributes exposing (class, href, style, title, value)
import Html.Events exposing (onClick)
import Types exposing (Msg(..))
import Url exposing (percentEncode)
import Utils.Filter
import Views.AlertList.Types exposing (AlertListMsg(..))
import Views.FilterBar.Types as FilterBarTypes
import Views.Shared.Alert exposing (annotation, annotationsButton, generatorUrlButton, titleView)
import Views.SilenceForm.Parsing exposing (newSilenceFromAlertLabels)


view : List ( String, String ) -> Maybe String -> GettableAlert -> Html Msg
view labels maybeActiveId alert =
    let
        -- remove the grouping labels, and bring the alertname to front
        ungroupedLabels =
            alert.labels
                |> Dict.toList
                |> List.filter ((\b a -> List.member a b) labels >> not)
                |> List.partition (Tuple.first >> (==) "alertname")
                |> (\( a, b ) -> a ++ b)
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
            , if Dict.size alert.annotations > 0 then
                annotationsButton maybeActiveId alert
                    |> Html.map (\msg -> MsgForAlertList (SetActive msg))

              else
                text ""
            , case alert.generatorURL of
                Just url ->
                    generatorUrlButton url

                Nothing ->
                    text ""
            , silenceButton alert
            , inhibitedIcon alert
            , mutedIcon alert
            , linkButton alert
            ]
        , if maybeActiveId == Just alert.fingerprint then
            table [ class "table w-100 mb-1" ] (List.map annotation <| Dict.toList alert.annotations)

          else
            text ""
        , div [] (List.map labelButton ungroupedLabels)
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


linkButton : GettableAlert -> Html Msg
linkButton alert =
    let
        link =
            alert.labels
                |> Dict.toList
                |> List.map (\( k, v ) -> Utils.Filter.Matcher k Utils.Filter.Eq v)
                |> Utils.Filter.stringifyFilter
                |> percentEncode
                |> (++) "#/alerts?filter="
    in
    a
        [ class "btn btn-outline-info border-0"
        , href link
        ]
        [ i [ class "fa fa-link mr-2" ] []
        , text "Link"
        ]


silenceButton : GettableAlert -> Html Msg
silenceButton alert =
    case List.head alert.status.silencedBy of
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


inhibitedIcon : GettableAlert -> Html Msg
inhibitedIcon alert =
    case List.head alert.status.inhibitedBy of
        Just _ ->
            span
                [ class "btn btn-outline-danger border-0"
                ]
                [ i [ class "fa fa-eye-slash mr-2" ] []
                , text "Inhibited"
                ]

        Nothing ->
            text ""


mutedIcon : GettableAlert -> Html Msg
mutedIcon alert =
    case List.head alert.status.mutedBy of
        Just _ ->
            span
                [ class "btn btn-outline-danger border-0"
                ]
                [ i [ class "fa fa-bell-slash mr-2" ] []
                , text "Muted"
                ]

        Nothing ->
            text ""
