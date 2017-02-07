module Silences.Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onClick, onInput)


-- Internal Imports

import Types exposing (Silence, Matcher, Msg(..))
import Utils.Views exposing (..)
import Utils.Date


silenceList : Silence -> Html Msg
silenceList silence =
    li
        [ class "pa3 pa4-ns bb b--black-10" ]
        [ silenceBase silence
        ]


silence : Silence -> Html Msg
silence silence =
    div []
        [ silenceBase silence
        , silenceExtra silence
        ]


silenceBase : Silence -> Html Msg
silenceBase silence =
    let
        -- TODO: Check with fabxc if the alert being in the first position can
        -- be relied upon.
        alertName =
            case List.head silence.matchers of
                Just m ->
                    m.value

                Nothing ->
                    ""

        editUrl =
            String.join "/" [ "#/silences", toString silence.id, "edit" ]
    in
        div [ class "f6 mb3" ]
            [ a
                [ class "db link blue mb3"
                , href ("#/silences/" ++ (toString silence.id))
                ]
                [ b [ class "db f4 mb1" ]
                    [ text alertName ]
                ]
            , div [ class "mb1" ]
                [ buttonLink "fa-pencil" editUrl "blue" Noop
                , buttonLink "fa-trash-o" "#/silences" "dark-red" (DestroySilence silence)
                , p [ class "dib mr2" ] [ text <| "Until " ++ Utils.Date.dateFormat silence.endsAt.t ]
                ]
            , div [ class "mb2 w-80-l w-100-m" ] (List.map matcherButton <| List.filter (\m -> m.name /= "alertname") silence.matchers)
            ]


silenceExtra : Silence -> Html msg
silenceExtra silence =
    let
        -- TODO: It would be nice to get this from the API. I want
        -- Alertmanager's view of things, not the browser client's.
        status =
            "elapsed"
    in
        div [ class "f6" ]
            [ div [ class "mb1" ]
                [ p []
                    [ text "Status: "
                    , Utils.Views.button "ph3 pv2" status
                    ]
                , div []
                    [ label [ class "f6 dib mb2 mr2 w-40" ] [ text "Created by" ]
                    , p [] [ text silence.createdBy ]
                    ]
                , div []
                    [ label [ class "f6 dib mb2 mr2 w-40" ] [ text "Comment" ]
                    , p [] [ text silence.comment ]
                    ]
                ]
            ]



-- TODO: Add field validations.


silenceForm : String -> Silence -> Html Msg
silenceForm kind silence =
    let
        base =
            "/#/silences/"

        url =
            case kind of
                "New" ->
                    base ++ "new"

                "Edit" ->
                    base ++ (toString silence.id) ++ "/edit"

                _ ->
                    "/#/silences"
    in
        div [ class "pa4 black-80" ]
            [ fieldset [ class "ba b--transparent ph0 mh0" ]
                [ legend [ class "ph0 mh0 fw6" ] [ text <| kind ++ " Silence" ]
                , formField "Start" (silence.startsAt.s) UpdateStartsAt
                , formField "End" (silence.endsAt.s) UpdateEndsAt
                , div [ class "mt3" ]
                    [ label [ class "f6 b db mb2" ]
                        [ text "Matchers "
                        , span [ class "normal black-60" ] [ text "Alerts affected by this silence. Format: name=value" ]
                        ]
                    , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Name" ]
                    , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Value" ]
                    ]
                , div [] <| List.map matcherForm silence.matchers
                , iconButtonMsg "blue" "fa-plus" AddMatcher
                , formField "Creator" silence.createdBy UpdateCreatedBy
                , textField "Comment" silence.comment UpdateComment
                , div [ class "mt3" ]
                    [ a [ class "f6 link br2 ba ph3 pv2 mr2 dib blue", onClick (CreateSilence silence) ] [ text "Create" ]
                      -- Reset isn't working for "New" -- just updates time.
                    , a [ class "f6 link br2 ba ph3 pv2 mb2 dib dark-red", href url ] [ text "Reset" ]
                    ]
                ]
            ]


matcherForm : Matcher -> Html Msg
matcherForm matcher =
    div []
        [ formInput matcher.name (UpdateMatcherName matcher)
        , formInput matcher.value (UpdateMatcherValue matcher)
        , checkbox "Regex" matcher.isRegex (UpdateMatcherRegex matcher)
        , iconButtonMsg "dark-red" "fa-trash-o" (DeleteMatcher matcher)
        ]


matcherButton : Matcher -> Html msg
matcherButton matcher =
    let
        join =
            if matcher.isRegex then
                "=~"
            else
                "="
    in
        Utils.Views.button "light-silver hover-black ph3 pv2" <| String.join join [ matcher.name, matcher.value ]
