module Float
    exposing
        ( epsilon
        , infinity
        , maxAbsValue
        , minAbsNormal
        , minAbsValue
        , nan
        )

{-| Float contains useful constants related to 64 bit floating point numbers,
as specified in IEEE 754-2008.

@docs epsilon, infinity, nan, minAbsNormal, minAbsValue, maxAbsValue

-}


{-| Largest possible rounding error in a single 64 bit floating point
calculation on an x86-x64 CPU. Also known as the Machine Epsilon.

If you do not know what tolerance you should use, use this number, multiplied
by the number of floating point operations you're doing in your calculation.

According to the [MSDN system.double.epsilon documentation]
(<https://msdn.microsoft.com/en-us/library/system.double.epsilon.aspx#Remarks>),
ARM has a machine epsilon that is too small to represent in a 64 bit float,
so we're simply ignoring that. On phones, tablets, raspberry pi's and other
devices with ARM chips, you might get slightly better precision than we assume
here.

-}
epsilon : Float
epsilon =
    2.0 ^ -52


{-| Positive infinity. Negative infinity is just -infinity.
-}
infinity : Float
infinity =
    1.0 / 0.0


{-| Not a Number. NaN does not compare equal to anything, including itself.
Any operation including NaN will result in NaN.
-}
nan : Float
nan =
    0.0 / 0.0


{-| Smallest possible value which still has full precision.

Values closer to zero are denormalized, which means that they are
using some of the significant bits to emulate a slightly larger mantissa.
The number of significant binary digits is proportional to the binary
logarithm of the denormalized number; halving a denormalized number also
halves the precision of that number.

-}
minAbsNormal : Float
minAbsNormal =
    2.0 ^ -1022


{-| Smallest absolute value representable in a 64 bit float.
-}
minAbsValue : Float
minAbsValue =
    2.0 ^ -1074


{-| Largest finite absolute value representable in a 64 bit float.
-}
maxAbsValue : Float
maxAbsValue =
    (2.0 - epsilon) * 2.0 ^ 1023
