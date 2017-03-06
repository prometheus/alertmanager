module Silences.Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onClick, onInput)
import Http exposing (Error)
import Silences.Types exposing (Silence, SilencesMsg(..), Msg(..), OutMsg(UpdateFilter), Route(..))
import Utils.Types exposing (Matcher, ApiResponse(..), Filter, ApiData)
import Utils.Views exposing (iconButtonMsg, checkbox, textField, formInput, formField, buttonLink, error, loading)
import Utils.Date
import ISO8601 exposing (toTime)


view : Route -> ApiData (List Silence) -> ApiData Silence -> ISO8601.Time -> Filter -> Html Msg
view route apiSilences apiSilence currentTime filter =
    case route of
        ShowSilences _ ->
            case apiSilences of
                Success sils ->
                    -- Add buttons at the top to filter Active/Pending/Expired
                    silences sils filter (text "")

                Loading ->
                    loading

                Failure msg ->
                    silences [] filter (error msg)

        ShowSilence name ->
            case apiSilence of
                Success sil ->
                    silence sil currentTime

                Loading ->
                    loading

                Failure msg ->
                    error msg

        ShowNewSilence ->
            case apiSilence of
                Success silence ->
                    silenceForm "New" silence

                Loading ->
                    loading

                Failure msg ->
                    error msg

        ShowEditSilence id ->
            case apiSilence of
                Success silence ->
                    silenceForm "Edit" silence

                Loading ->
                    loading

                Failure msg ->
                    error msg


silences : List Silence -> Filter -> Html Msg -> Html Msg
silences silences filter errorHtml =
    let
        filterText =
            Maybe.withDefault "" filter.text

        html =
            if List.isEmpty silences then
                div [ class "mt2" ] [ text "no silences found" ]
            else
                ul
                    [ classList
                        [ ( "list", True )
                        , ( "pa0", True )
                        ]
                    ]
                    (List.map silenceList silences)
    in
        div []
            [ Html.map ForParent (textField "Filter" filterText (UpdateFilter filter))
            , a [ class "f6 link br2 ba ph3 pv2 mr2 dib blue", onClick (ForSelf FilterSilences) ] [ text "Filter Silences" ]
            , a [ class "f6 link br2 ba ph3 pv2 mr2 dib blue", href "#/silences/new" ] [ text "New Silence" ]
            , errorHtml
            , html
            ]


silenceList : Silence -> Html Msg
silenceList silence =
    li
        [ class "pa3 pa4-ns bb b--black-10" ]
        [ silenceBase silence
        ]


silence : Silence -> ISO8601.Time -> Html Msg
silence silence currentTime =
    div []
        [ silenceBase silence
        , silenceExtra silence currentTime
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
            String.join "/" [ "#/silences", silence.id, "edit" ]
    in
        div [ class "f6 mb3" ]
            [ a
                [ class "db link blue mb3"
                , href ("#/silences/" ++ silence.id)
                ]
                [ b [ class "db f4 mb1" ]
                    [ text alertName ]
                ]
            , div [ class "mb1" ]
                [ buttonLink "fa-pencil" editUrl "blue" (ForSelf Noop)
                , buttonLink "fa-trash-o" "#/silences" "dark-red" (ForSelf (DestroySilence silence))
                , p [ class "dib mr2" ] [ text <| "Until " ++ Utils.Date.dateFormat silence.endsAt.t ]
                ]
            , div [ class "mb2 w-80-l w-100-m" ] (List.map matcherButton <| List.filter (\m -> m.name /= "alertname") silence.matchers)
            ]


silenceExtra : Silence -> ISO8601.Time -> Html msg
silenceExtra silence currentTime =
    div [ class "f6" ]
        [ div [ class "mb1" ]
            [ p []
                [ text "Status: "
                , Utils.Views.button "ph3 pv2" (status silence currentTime)
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


status : Silence -> ISO8601.Time -> String
status silence currentTime =
    let
        ct =
            toTime currentTime

        et =
            toTime silence.endsAt.t

        st =
            toTime silence.startsAt.t
    in
        if et <= ct then
            "expired"
        else if st > ct then
            "pending"
        else
            "active"


silenceForm : String -> Silence -> Html Msg
silenceForm kind silence =
    -- TODO: Add field validations.
    let
        base =
            "/#/silences/"

        boundMatcherForm =
            matcherForm silence

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
                , (formField "Start" silence.startsAt.s (UpdateStartsAt silence))
                , (formField "End" silence.endsAt.s (UpdateEndsAt silence))
                , div [ class "mt3" ]
                    [ label [ class "f6 b db mb2" ]
                        [ text "Matchers "
                        , span [ class "normal black-60" ] [ text "Alerts affected by this silence. Format: name=value" ]
                        ]
                    , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Name" ]
                    , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Value" ]
                    ]
                , div [] (List.map boundMatcherForm silence.matchers)
                , iconButtonMsg "blue" "fa-plus" (AddMatcher silence)
                , formField "Creator" silence.createdBy (UpdateCreatedBy silence)
                , textField "Comment" silence.comment (UpdateComment silence)
                , div [ class "mt3" ]
                    [ a [ class "f6 link br2 ba ph3 pv2 mr2 dib blue", onClick (CreateSilence silence) ] [ text "Create" ]
                      -- Reset isn't working for "New" -- just updates time.
                    , a [ class "f6 link br2 ba ph3 pv2 mb2 dib dark-red", href url ] [ text "Reset" ]
                    ]
                ]
            ]
            |> (Html.map ForSelf)


matcherForm : Silence -> Matcher -> Html SilencesMsg
matcherForm silence matcher =
    div []
        [ formInput matcher.name (UpdateMatcherName silence matcher)
        , formInput matcher.value (UpdateMatcherValue silence matcher)
        , checkbox "Regex" matcher.isRegex (UpdateMatcherRegex silence matcher)
        , iconButtonMsg "dark-red" "fa-trash-o" (DeleteMatcher silence matcher)
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
