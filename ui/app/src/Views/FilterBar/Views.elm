module Views.FilterBar.Views exposing (view)

import Html exposing (Html, a, button, div, i, input, small, span, text)
import Html.Attributes exposing (class, disabled, href, id, style, value)
import Html.Events exposing (onClick, onInput)
import Utils.Filter exposing (Matcher, convertFilterMatcher)
import Utils.Keyboard exposing (onKeyDown, onKeyUp)
import Utils.List
import Views.FilterBar.Types exposing (Model, Msg(..))
import Views.SilenceForm.Parsing exposing (newSilenceFromMatchers)


keys :
    { backspace : Int
    , enter : Int
    }
keys =
    { backspace = 8
    , enter = 13
    }


viewMatcher : Matcher -> Html Msg
viewMatcher matcher =
    div [ class "col col-auto" ]
        [ div [ class "btn-group mr-2 mb-2" ]
            [ button
                [ class "btn btn-outline-info"
                , onClick (DeleteFilterMatcher True matcher)
                ]
                [ text <| Utils.Filter.stringifyMatcher matcher
                ]
            , button
                [ class "btn btn-outline-danger"
                , onClick (DeleteFilterMatcher False matcher)
                ]
                [ text "×" ]
            ]
        ]


viewMatchers : List Matcher -> List (Html Msg)
viewMatchers matchers =
    matchers
        |> List.map viewMatcher


view : { showSilenceButton : Bool } -> Model -> Html Msg
view { showSilenceButton } { matchers, matcherText, backspacePressed } =
    let
        maybeMatcher =
            Utils.Filter.parseMatcher matcherText

        maybeLastMatcher =
            Utils.List.lastElem matchers

        className =
            if matcherText == "" then
                ""

            else
                case maybeMatcher of
                    Just _ ->
                        "has-success"

                    Nothing ->
                        "has-danger"

        keyDown key =
            if key == keys.enter then
                maybeMatcher
                    |> Maybe.map (AddFilterMatcher True)
                    |> Maybe.withDefault Noop

            else if key == keys.backspace then
                if matcherText == "" then
                    case ( backspacePressed, maybeLastMatcher ) of
                        ( False, Just lastMatcher ) ->
                            DeleteFilterMatcher True lastMatcher

                        _ ->
                            Noop

                else
                    PressingBackspace True

            else
                Noop

        keyUp key =
            if key == keys.backspace then
                PressingBackspace False

            else
                Noop

        onClickAttr =
            maybeMatcher
                |> Maybe.map (AddFilterMatcher True)
                |> Maybe.withDefault Noop
                |> onClick

        isDisabled =
            maybeMatcher == Nothing

        dataMatchers =
            matchers
                |> List.map convertFilterMatcher
    in
    div
        [ class "row no-gutters align-items-start" ]
        (viewMatchers matchers
            ++ [ div
                    [ class ("col " ++ className)
                    , style "min-width"
                        (if showSilenceButton then
                            "300px"

                         else
                            "200px"
                        )
                    ]
                    [ div [ class "row no-gutters align-content-stretch" ]
                        [ div [ class "col input-group" ]
                            [ input
                                [ id "filter-bar-matcher"
                                , class "form-control"
                                , value matcherText
                                , onKeyDown keyDown
                                , onKeyUp keyUp
                                , onInput UpdateMatcherText
                                ]
                                []
                            , span
                                [ class "input-group-btn" ]
                                [ button [ class "btn btn-primary", disabled isDisabled, onClickAttr ] [ text "+" ] ]
                            ]
                        , if showSilenceButton then
                            div [ class "col col-auto input-group-btn ml-2" ]
                                [ div [ class "input-group" ]
                                    [ a
                                        [ class "btn btn-outline-info"
                                        , href (newSilenceFromMatchers dataMatchers)
                                        ]
                                        [ i [ class "fa fa-bell-slash-o mr-2" ] []
                                        , text "Silence"
                                        ]
                                    ]
                                ]

                          else
                            text ""
                        ]
                    , small [ class "form-text text-muted" ]
                        [ text "Custom matcher, e.g."
                        , button
                            [ class "btn btn-link btn-sm align-baseline"
                            , onClick (UpdateMatcherText exampleMatcher)
                            ]
                            [ text exampleMatcher ]
                        ]
                    ]
               ]
        )


exampleMatcher : String
exampleMatcher =
    "env=\"production\""
