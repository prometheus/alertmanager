module Views.Shared.AlertCompact exposing (annotation, annotationsButton, generatorUrlButton, titleView, view)

import Alerts.Types exposing (Alert)
import Html exposing (Html, a, button, div, i, li, span, table, td, text, th, tr)
import Html.Attributes exposing (class, href, style)
import Html.Events exposing (onClick)
import Utils.Date
import Utils.Views exposing (labelButton)
import Views.Shared.Types exposing (Msg(..))


view : Maybe String -> Alert -> Html Msg
view maybeActiveId alert =
    let
        -- remove the grouping labels, and bring the alertname to front
        ungroupedLabels =
            alert.labels
                |> List.partition (Tuple.first >> (==) "alertname")
                |> (\( a, b ) -> (++) a b)
                |> List.map (\( a, b ) -> String.join "=" [ a, b ])
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
            , if List.length alert.annotations > 0 then
                annotationsButton maybeActiveId alert

              else
                text ""
            , generatorUrlButton alert.generatorUrl
            ]
        , if maybeActiveId == Just alert.id then
            table
                [ class "table w-100 mb-1" ]
                (List.map annotation alert.annotations)

          else
            text ""
        , div [] (List.map (labelButton Nothing) ungroupedLabels)
        ]


annotationsButton : Maybe String -> Alert -> Html Msg
annotationsButton maybeActiveId alert =
    if maybeActiveId == Just alert.id then
        button
            [ onClick (OptionalValue Nothing)
            , class "btn btn-outline-info border-0 active"
            ]
            [ i [ class "fa fa-minus mr-2" ] [], text "Info" ]

    else
        button
            [ class "btn btn-outline-info border-0"
            , onClick (OptionalValue (Just alert.id))
            ]
            [ i [ class "fa fa-plus mr-2" ] [], text "Info" ]


annotation : ( String, String ) -> Html msg
annotation ( key, value ) =
    tr []
        [ th [ class "text-nowrap" ] [ text (key ++ ":") ]
        , td [ class "w-100" ] (Utils.Views.linkifyText value)
        ]


titleView : Alert -> Html msg
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
            (Utils.Date.dateTimeFormat startsAt
                ++ inhibited
            )
        ]


generatorUrlButton : String -> Html msg
generatorUrlButton url =
    a
        [ class "btn btn-outline-info border-0", href url ]
        [ i [ class "fa fa-line-chart mr-2" ] []
        , text "Source"
        ]
